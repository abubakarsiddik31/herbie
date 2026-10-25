package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/rag"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/tool"
)

// botAgentAPI is chat.Agent narrowed to the run surface the background
// runner calls; tests swap in scriptable fakes, production gets deps.Agent.
type botAgentAPI interface {
	Run(ctx context.Context, deps chat.Deps, history []model.Message, prompt string, parts []model.Part, sink chat.Sink, tools []tool.Tool[chat.Deps], spec chat.RunSpec) (chat.Outcome, error)
	RunDeferred(ctx context.Context, deps chat.Deps, history []model.Message, res golem.DeferredResults, sink chat.Sink, tools []tool.Tool[chat.Deps], spec chat.RunSpec) (chat.Outcome, error)
}

// BotRunStore is the bot-run queue surface; *storage.BotRuns satisfies it
// (pin below), tests run against in-memory fakes.
type BotRunStore interface {
	Create(ctx context.Context, run storage.BotRun) (storage.BotRun, error)
	Get(ctx context.Context, userID, id string) (storage.BotRun, error)
	ListByBot(ctx context.Context, userID, botID string, limit int) ([]storage.BotRun, error)
	LatestPerBot(ctx context.Context, userID string) (map[string]storage.BotRun, error)
	ClaimNext(ctx context.Context) (*storage.BotRun, error)
	SetWaitingApproval(ctx context.Context, id string) error
	SetFinished(ctx context.Context, id, status string, inputTokens, outputTokens, requests int, costMicroUsd int64, runErr string) error
	UpdateActivity(ctx context.Context, id string, activity []byte) error
	RecoverRunning(ctx context.Context) error
	HasActiveForConversation(ctx context.Context, userID, conversationID string) (bool, error)
}

var _ BotRunStore = (*storage.BotRuns)(nil)

const (
	// maxBotPauseCycles caps how often one run may park on approvals before
	// the runner gives up — prevents an approve/pause ping-pong loop.
	maxBotPauseCycles = 5
	// botActivityFlushInterval is the live-progress cadence for the run's
	// activity journal while it executes (pause/finish/cancel flush too).
	botActivityFlushInterval = 2 * time.Second
)

// botRunner drains the bot run queue in-process. One goroutine per active
// run; a slot channel caps how many runs are executing OR parked for
// approval at once (parked runs hold their slot — a parked run is still
// live work). cancels/resumes are the run-ID registries behind
// CancelBotRun/NotifyBotRunResume; both are mutex-guarded.
type botRunner struct {
	s           *Server
	concurrency int
	poll        time.Duration
	wake        chan struct{}
	mu          sync.Mutex
	cancels     map[string]context.CancelFunc
	resumes     map[string]chan struct{}
}

// StartBotRunner boots the queue: crash recovery first, then the claim
// loop. Recovery requeues runs stranded in running AND waiting_approval —
// a parked run's resume channel dies with the old process, so the next
// boot cannot resume it in place. Instead the requeued run restarts FRESH:
// execute() deletes the conversation's still-pending approval rows and
// re-runs from persisted history (golem history already contains the
// tool-call messages, so the fresh run re-asks whatever it still needs).
// Idempotent; a nil BotRuns store is a no-op (feature unwired).
func (s *Server) StartBotRunner(parent context.Context) {
	if s.runner.Load() != nil || s.deps.BotRuns == nil {
		return
	}
	r := &botRunner{
		s:           s,
		concurrency: s.deps.Cfg.BotRunnerConcurrency,
		poll:        s.deps.Cfg.BotRunnerPollInterval,
		wake:        make(chan struct{}, 1),
		cancels:     map[string]context.CancelFunc{},
		resumes:     map[string]chan struct{}{},
	}
	if r.concurrency <= 0 {
		r.concurrency = 2
	}
	if r.poll <= 0 {
		r.poll = 2 * time.Second
	}
	s.runner.Store(r)
	if err := s.deps.BotRuns.RecoverRunning(parent); err != nil {
		s.deps.Log.Error("bot run recover", "err", err)
	}
	go r.loop(parent)
	// The routine scheduler rides the same lifecycle (and wake channel) as
	// the claim loop: one parent context, two goroutines.
	s.StartRoutineScheduler(parent)
}

// WakeBotRunner nudges the claim loop immediately (enqueue paths call it;
// the poll ticker is only the fallback cadence).
func (s *Server) WakeBotRunner() {
	r := s.runner.Load()
	if r == nil {
		return
	}
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// CancelBotRun cancels a live run (executing or parked). Settlement is up
// to the run's goroutine: it persists partial evidence and finishes the
// row as cancelled — unless the parent context is also done, in which
// case the row is left for the next boot's RecoverRunning to requeue.
func (s *Server) CancelBotRun(runID string) {
	r := s.runner.Load()
	if r == nil {
		return
	}
	r.mu.Lock()
	c, ok := r.cancels[runID]
	if ok {
		delete(r.cancels, runID)
	}
	r.mu.Unlock()
	if ok {
		c()
	}
}

// NotifyBotRunResume wakes the goroutine parked on runID. The channel is
// removed from the registry under the lock before closing, so a second
// notify (or one racing the run's exit) is a safe no-op.
func (s *Server) NotifyBotRunResume(runID string) {
	r := s.runner.Load()
	if r == nil {
		return
	}
	r.mu.Lock()
	ch, ok := r.resumes[runID]
	if ok {
		delete(r.resumes, runID)
	}
	r.mu.Unlock()
	if ok {
		close(ch)
	}
}

func (r *botRunner) setCancel(runID string, cancel context.CancelFunc) {
	r.mu.Lock()
	r.cancels[runID] = cancel
	r.mu.Unlock()
}

func (r *botRunner) clearCancel(runID string) {
	r.mu.Lock()
	delete(r.cancels, runID)
	r.mu.Unlock()
}

func (r *botRunner) setResume(runID string, ch chan struct{}) {
	r.mu.Lock()
	r.resumes[runID] = ch
	r.mu.Unlock()
}

func (r *botRunner) clearResume(runID string) {
	r.mu.Lock()
	delete(r.resumes, runID)
	r.mu.Unlock()
}

// loop claims queued runs until ctx dies. Claiming stops the moment every
// slot is held (busy or parked); a finishing run's release combines with
// the wake/tick cadence to pick the next one up.
func (r *botRunner) loop(ctx context.Context) {
	slots := make(chan struct{}, r.concurrency)
	tick := time.NewTicker(r.poll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.wake:
		case <-tick.C:
		}
	claim:
		for {
			// Take a slot BEFORE claiming so parked runs (which hold
			// theirs indefinitely) really do consume concurrency.
			select {
			case slots <- struct{}{}:
			default:
				break claim
			}
			run, err := r.s.deps.BotRuns.ClaimNext(ctx)
			if err != nil {
				<-slots
				r.s.deps.Log.Error("bot run claim", "err", err)
				break claim
			}
			if run == nil { // queue empty
				<-slots
				break claim
			}
			go func(run *storage.BotRun) {
				defer func() { <-slots }()
				r.execute(ctx, run)
			}(run)
		}
	}
}

// runSink buffers a background run: deltas accumulate, tool events build a
// short activity trace the UI polls (capped, newest 50). Implements
// chat.Sink; safe for the flusher goroutine and the run goroutine at once.
type runSink struct {
	mu            sync.Mutex
	buf           strings.Builder
	activity      []map[string]any
	inTok, outTok int
}

func (s *runSink) Delta(t string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf.WriteString(t)
	return nil
}

func (s *runSink) ModelStart() {}

func (s *runSink) ModelEnd(in, out int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inTok += in
	s.outTok += out
}

func (s *runSink) ToolStart(name string) {
	s.append(map[string]any{"event": "tool_start", "name": name, "t": time.Now().UnixMilli()})
}

func (s *runSink) ToolEnd(name string, ok bool) {
	s.append(map[string]any{"event": "tool_end", "name": name, "ok": ok, "t": time.Now().UnixMilli()})
}

func (s *runSink) append(e map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activity = append(s.activity, e)
	if len(s.activity) > 50 {
		s.activity = s.activity[len(s.activity)-50:]
	}
}

// snapshot copies the sink's state for persistence/inspection.
func (s *runSink) snapshot() (string, []map[string]any, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	activity := append([]map[string]any(nil), s.activity...)
	return s.buf.String(), activity, s.inTok, s.outTok
}

// flushActivity persists the sink's activity journal (best-effort; the
// background context keeps working while the run's ctx tears down).
func (r *botRunner) flushActivity(runID string, sink *runSink) {
	_, activity, _, _ := sink.snapshot()
	if len(activity) == 0 {
		return
	}
	raw, err := json.Marshal(activity)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = r.s.deps.BotRuns.UpdateActivity(ctx, runID, raw)
}

// execute runs one claimed bot run to a terminal status. The goroutine
// holds a queue slot for its whole life, parks included.
func (r *botRunner) execute(parent context.Context, run *storage.BotRun) {
	s := r.s
	// Settlement writes must survive the run's own cancellation.
	settle := context.WithoutCancel(parent)
	// Early-setup failures settle "failed" — EXCEPT on shutdown: these calls
	// still run on parent, and a cancelled parent (e.g. pgx returning
	// context.Canceled) must not terminally fail the row; it stays
	// running/waiting_approval for the next boot's RecoverRunning requeue,
	// mirroring settleFailure's parent check.
	fail := func(msg string) {
		if parent.Err() != nil {
			return
		}
		_ = s.deps.BotRuns.SetFinished(settle, run.ID, "failed", 0, 0, 0, 0, msg)
	}

	// Fresh start: a queued run always begins from persisted history, so
	// still-pending approval rows (left by this run's parked ancestor
	// after a restart — see StartBotRunner) must go: the fresh run re-asks
	// what it needs, and stale rows would block the conversation's live
	// send path (blockedByPendingApproval).
	if s.deps.Pending != nil {
		if err := s.deps.Pending.DeletePendingFor(parent, run.ConversationID, run.UserID); err != nil {
			s.deps.Log.Error("bot run clear pending", "err", err, "run_id", run.ID)
		}
	}

	conv, err := s.deps.Convos.ByID(parent, run.ConversationID, run.UserID)
	if err != nil {
		fail("conversation not available: " + err.Error())
		return
	}
	spec := s.runSpecFor(parent, conv)
	// A bot-scoped run threads its live bot through so the workspace file
	// tools register and chat.Deps carries the bot identity (plain
	// conversations pass nil).
	bot := s.botFor(parent, run.UserID, conv.BotID)
	tools, err := s.userTools(parent, run.UserID, spec.RagEnabled, bot)
	if err != nil {
		fail("could not load tools: " + err.Error())
		return
	}

	// The task is the turn's user row; history is everything before it —
	// exactly the send path's split (handleSendMessage + historyFrom).
	if err := s.deps.Msgs.Add(parent, storage.Message{
		ConversationID: run.ConversationID, UserID: run.UserID,
		Role: string(model.RoleUser), Content: run.Task,
		Data: mustJSON(model.Message{Role: model.RoleUser, Content: run.Task}),
	}); err != nil {
		fail("could not persist task: " + err.Error())
		return
	}
	rows, err := s.deps.Msgs.ForConversation(parent, run.ConversationID, run.UserID)
	if err != nil {
		fail("could not load history: " + err.Error())
		return
	}
	known := knownRows(rows)

	var sources []rag.Scored
	var sourcesMu sync.Mutex
	deps := s.buildDeps(run.UserID, run.ConversationID, &sources, &sourcesMu, bot)

	sink := &runSink{}
	runCtx, cancelRun := context.WithCancel(parent)
	r.setCancel(run.ID, cancelRun)
	defer r.clearCancel(run.ID)
	defer cancelRun()

	// Live activity flusher alongside the run; stopped at exit, with a
	// final flush deferred after every settle path below.
	flushStop := make(chan struct{})
	go func() {
		t := time.NewTicker(botActivityFlushInterval)
		defer t.Stop()
		for {
			select {
			case <-flushStop:
				return
			case <-t.C:
				r.flushActivity(run.ID, sink)
			}
		}
	}()
	defer close(flushStop)
	defer r.flushActivity(run.ID, sink)

	// Usage totals across pause cycles feed SetFinished.
	var totalIn, totalOut, totalReqs int
	cost := func() int64 {
		return s.deps.Rates.ChatCostMicrosFor(spec.Model, totalIn, totalOut)
	}

	outcome, err := s.botAgent.Run(runCtx, deps, historyFrom(rows, len(rows)-1), run.Task, nil, sink, tools, spec)
	deferred := false // cycle 0 persists positionally; resumes use payload dedupe
	pauses := 0

	for {
		if err != nil {
			r.settleFailure(settle, parent, run, spec, err, totalIn, totalOut, totalReqs, cost())
			return
		}
		newFrom := 0
		if !deferred {
			newFrom = replayedPrefixLen(outcome.Messages)
		}

		if len(outcome.Pending) == 0 {
			// Success: persist the run's rows, meter, finish.
			if _, perr := s.persistRunMessages(settle, run.UserID, run.ConversationID, spec.Model,
				outcome.Messages, outcome.Usage, outcome.Requests, false, known, newFrom, sourceJSON(sources)); perr != nil {
				s.deps.Log.Error("persist bot messages", "err", perr, "run_id", run.ID)
			}
			if uerr := s.deps.Usage.Add(settle, usageEventFor(run.UserID, run.ConversationID, spec.Model,
				outcome.Usage, outcome.Requests, s.deps.Rates)); uerr != nil {
				s.deps.Log.Error("record bot usage", "err", uerr, "run_id", run.ID)
			}
			_ = s.deps.Convos.Touch(settle, run.ConversationID)
			totalIn += outcome.Usage.InputTokens
			totalOut += outcome.Usage.OutputTokens
			totalReqs += outcome.Requests
			_ = s.deps.BotRuns.SetFinished(settle, run.ID, "succeeded", totalIn, totalOut, totalReqs, cost(), "")
			return
		}

		pauses++
		if pauses > maxBotPauseCycles {
			_ = s.deps.BotRuns.SetFinished(settle, run.ID, "failed", totalIn, totalOut, totalReqs, cost(),
				"too many approval cycles")
			return
		}

		// Pause: mirror pauseRun's DB half — persist through the
		// unanswered tool call, meter the partial cycle, park the calls
		// (with their persisted, conversation-unique call IDs), mark the
		// run waiting_approval.
		totalIn += outcome.Usage.InputTokens
		totalOut += outcome.Usage.OutputTokens
		totalReqs += outcome.Requests
		idMap, perr := s.persistRunMessages(settle, run.UserID, run.ConversationID, spec.Model,
			outcome.Messages, outcome.Usage, outcome.Requests, false, known, newFrom, nil)
		if perr != nil {
			s.deps.Log.Error("persist paused bot messages", "err", perr, "run_id", run.ID)
		}
		if uerr := s.deps.Usage.Add(settle, usageEventFor(run.UserID, run.ConversationID, spec.Model,
			outcome.Usage, outcome.Requests, s.deps.Rates)); uerr != nil {
			s.deps.Log.Error("record bot usage", "err", uerr, "run_id", run.ID)
		}
		calls := make([]storage.PendingToolCall, 0, len(outcome.Pending))
		for _, p := range outcome.Pending {
			callID := p.CallID
			if mapped, ok := idMap[p.CallID]; ok {
				callID = mapped
			}
			calls = append(calls, storage.PendingToolCall{
				CallID: callID, ConversationID: run.ConversationID, UserID: run.UserID,
				ToolName: p.ToolName, Args: p.Args, Reason: p.Reason, Status: "pending",
			})
		}
		if aerr := s.deps.Pending.Add(settle, calls); aerr != nil {
			s.deps.Log.Error("persist pending calls", "err", aerr, "run_id", run.ID)
		}
		_ = s.deps.Convos.Touch(settle, run.ConversationID)
		if werr := s.deps.BotRuns.SetWaitingApproval(settle, run.ID); werr != nil {
			s.deps.Log.Error("park bot run", "err", werr, "run_id", run.ID)
		}
		r.flushActivity(run.ID, sink)

		// Park, holding the slot. Shutdown leaves the row in
		// waiting_approval for the next boot's RecoverRunning requeue;
		// a user cancel settles it as cancelled (messages already
		// persisted above). The resume channel is registered BEFORE the
		// decision check and stays registered across it: a decision flip
		// + notify that lands while the check runs (or after a premature
		// wake found calls still pending) necessarily overlaps a live
		// registration, so it closes the channel instead of being
		// dropped — no lost-wakeup window between check and re-park.
		resumed := false
		for !resumed {
			ch := make(chan struct{})
			r.setResume(run.ID, ch)
			pend, perr := s.deps.Pending.ForConversation(runCtx, run.ConversationID, run.UserID)
			if perr != nil {
				r.clearResume(run.ID)
				err = perr
				resumed = true // leave the wait; loop top settles the error
				break
			}
			if len(pend) == 0 {
				// All decided. Unregister and drain a notify that may have
				// raced the final decision flip; the signal is spent.
				r.clearResume(run.ID)
				select {
				case <-ch:
				default:
				}
				resumed = true
				break
			}
			select {
			case <-ch: // NotifyBotRunResume already unregistered it; re-check
			case <-runCtx.Done():
				r.clearResume(run.ID)
				if parent.Err() != nil {
					return // shutdown; nothing to settle
				}
				_ = s.deps.BotRuns.SetFinished(settle, run.ID, "cancelled", totalIn, totalOut, totalReqs, cost(), "")
				return
			}
			// Woke with the channel unregistered: loop re-registers a
			// fresh one before the next decision check.
		}
		if err != nil {
			continue
		}

		// Decisions: rows the approvals endpoint flipped before notifying.
		// Decision reasons live only in the audit log (the rows carry no
		// decision-reason column), so the resolutions map carries the
		// approvals alone.
		all, aerr := s.deps.Pending.ListAll(runCtx, run.ConversationID, run.UserID)
		if aerr != nil {
			err = aerr
			continue
		}
		resolutions := make(map[string]golem.Approval, len(all))
		for _, c := range all {
			switch c.Status {
			case "approved":
				resolutions[c.CallID] = golem.Approval{Approved: true}
			case "denied":
				resolutions[c.CallID] = golem.Approval{Approved: false}
			}
		}

		// Resume exactly like the SSE approvals path: paused-run history
		// (from the last user message on), payload-known dedupe, newFrom 0.
		rows, err = s.deps.Msgs.ForConversation(runCtx, run.ConversationID, run.UserID)
		if err != nil {
			continue
		}
		known = knownRows(rows)
		deferred = true
		outcome, err = s.botAgent.RunDeferred(runCtx, deps, pausedRunHistory(rows),
			golem.DeferredResults{Approvals: resolutions}, sink, tools, spec)
	}
}

// settleFailure classifies a run error and finishes the row. A parent
// cancellation (server shutdown) settles nothing: the row stays
// running/waiting_approval and the next boot's RecoverRunning requeues it
// for a fresh start. Context cancellations are user cancels; everything
// else is a failure carrying the golem.RunError message.
func (r *botRunner) settleFailure(settle, parent context.Context, run *storage.BotRun,
	spec chat.RunSpec, runErr error, totalIn, totalOut, totalReqs int, costMicros int64) {
	if parent.Err() != nil {
		return
	}
	s := r.s
	s.persistBotFailure(settle, run.UserID, run.ConversationID, spec, runErr)
	status, msg := "failed", runErr.Error()
	if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
		status, msg = "cancelled", ""
	}
	_ = s.deps.BotRuns.SetFinished(settle, run.ID, status, totalIn, totalOut, totalReqs, costMicros, msg)
}

// persistBotFailure stores a failed/cancelled background run's partial
// evidence (truncated), mirroring persistFailure's DB half minus the SSE.
func (s *Server) persistBotFailure(ctx context.Context, userID, convID string, spec chat.RunSpec, runErr error) {
	var runError *golem.RunError
	_ = errors.As(runErr, &runError)
	if runError != nil && runError.Partial != nil && len(runError.Partial.Messages) > 0 {
		if _, err := s.persistRunMessages(ctx, userID, convID, spec.Model, runError.Partial.Messages,
			runError.Partial.Usage, runError.Partial.Requests, true, nil,
			replayedPrefixLen(runError.Partial.Messages), nil); err != nil {
			s.deps.Log.Error("persist bot partial", "err", err, "conv_id", convID)
		}
		if uerr := s.deps.Usage.Add(ctx, usageEventFor(userID, convID, spec.Model,
			runError.Partial.Usage, runError.Partial.Requests, s.deps.Rates)); uerr != nil {
			s.deps.Log.Error("record bot usage", "err", uerr, "conv_id", convID)
		}
	}
	_ = s.deps.Convos.Touch(ctx, convID)
}

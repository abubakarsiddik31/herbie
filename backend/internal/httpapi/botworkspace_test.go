package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

// fakeBotFiles mirrors storage.BotFiles in memory (keyed by bot+path,
// user-scoped) for the workspace endpoints and the FS adapter.
type fakeBotFiles struct{ rows map[string]storage.BotFile }

func newFakeBotFiles() *fakeBotFiles { return &fakeBotFiles{rows: map[string]storage.BotFile{}} }

var _ BotFileStore = (*fakeBotFiles)(nil)

func (f *fakeBotFiles) key(botID, path string) string { return botID + "\x00" + path }

func (f *fakeBotFiles) Upsert(_ context.Context, file storage.BotFile) (storage.BotFile, error) {
	prev, ok := f.rows[f.key(file.BotID, file.Path)]
	if ok {
		file.ID, file.CreatedAt = prev.ID, prev.CreatedAt
	} else {
		file.ID = fmt.Sprintf("bf-%d", len(f.rows)+1)
		file.CreatedAt = time.Now()
	}
	file.SizeBytes = int64(len(file.Content))
	file.UpdatedAt = time.Now()
	f.rows[f.key(file.BotID, file.Path)] = file
	return file, nil
}

func (f *fakeBotFiles) Get(_ context.Context, userID, botID, path string) (storage.BotFile, error) {
	file, ok := f.rows[f.key(botID, path)]
	if !ok || file.UserID != userID {
		return storage.BotFile{}, storage.ErrNotFound
	}
	return file, nil
}

func (f *fakeBotFiles) List(_ context.Context, userID, botID string) ([]storage.BotFile, error) {
	var out []storage.BotFile
	for _, file := range f.rows {
		if file.BotID == botID && file.UserID == userID {
			file.Content = "" // storage.List elides content
			out = append(out, file)
		}
	}
	return out, nil
}

func (f *fakeBotFiles) Delete(_ context.Context, userID, botID, path string) error {
	file, ok := f.rows[f.key(botID, path)]
	if !ok || file.UserID != userID {
		return storage.ErrNotFound
	}
	delete(f.rows, f.key(botID, path))
	return nil
}

// TestUserToolsBotFileTools pins the single registration point: a bot run's
// tool list carries the four workspace tools, a plain conversation's does not.
func TestUserToolsBotFileTools(t *testing.T) {
	srv := &Server{deps: ServerDeps{
		Cfg:         config.Config{MaxBotsPerUser: 20},
		BotMemories: newFakeBotMemories(),
		BotFiles:    newFakeBotFiles(),
	}}
	bot := &storage.Bot{ID: "b-1", UserID: "u-1", Name: "Scout"}

	for _, mode := range []struct {
		name string
		bot  *storage.Bot
		want bool
	}{
		{"bot conversation", bot, true},
		{"plain conversation", nil, false},
	} {
		tools, err := srv.userTools(context.Background(), "u-1", true, mode.bot)
		if err != nil {
			t.Fatalf("%s: userTools: %v", mode.name, err)
		}
		got := map[string]bool{}
		for _, tl := range tools {
			got[tl.Name] = true
		}
		for _, want := range []string{
			chat.WriteFileToolName, chat.ReadFileToolName, chat.ListFilesToolName, chat.DeleteFileToolName,
		} {
			if got[want] != mode.want {
				t.Fatalf("%s: tool %s present=%v, want %v", mode.name, want, got[want], mode.want)
			}
		}
	}

	// No workspace store wired (BotFiles nil): even a bot run registers no
	// file tools rather than tools that cannot execute.
	srv.deps.BotFiles = nil
	tools, err := srv.userTools(context.Background(), "u-1", true, bot)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range tools {
		if tl.Name == chat.WriteFileToolName {
			t.Fatal("write_file registered without a BotFiles store")
		}
	}
}

// TestBotFSAdapter covers the storage→chat.BotFileStore adaptation round
// trip (read back what was written; list elides content).
func TestBotFSAdapter(t *testing.T) {
	fs := newFakeBotFiles()
	adapter := botFSAdapter{fs: fs, userID: "u-1"}
	if err := adapter.Write(context.Background(), "b-1", "notes/plan.md", "hello"); err != nil {
		t.Fatal(err)
	}
	content, err := adapter.Read(context.Background(), "b-1", "notes/plan.md")
	if err != nil || content != "hello" {
		t.Fatalf("read: %q %v", content, err)
	}
	files, err := adapter.List(context.Background(), "b-1")
	if err != nil || len(files) != 1 || files[0].Path != "notes/plan.md" || files[0].Size != 5 {
		t.Fatalf("list: %+v %v", files, err)
	}
	if err := adapter.Delete(context.Background(), "b-1", "notes/plan.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Read(context.Background(), "b-1", "notes/plan.md"); err != storage.ErrNotFound {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
}

func TestListBotFilesEndpoint(t *testing.T) {
	files := newFakeBotFiles()
	bots := newFakeBots()
	bot, err := bots.Create(context.Background(), storage.Bot{UserID: "u-1", Name: "Scout"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := files.Upsert(context.Background(), storage.BotFile{
		BotID: bot.ID, UserID: "u-1", Path: "notes/plan.md", Content: "hello world",
	}); err != nil {
		t.Fatal(err)
	}
	srv := &Server{deps: ServerDeps{Bots: bots, BotFiles: files}}

	rec := httptest.NewRecorder()
	req := authedJSONReq(http.MethodGet, "/api/bots/x/files", nil, "u-1")
	req.SetPathValue("id", bot.ID)
	srv.handleListBotFiles(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Files []struct {
			Path      string `json:"path"`
			SizeBytes int64  `json:"sizeBytes"`
			UpdatedAt string `json:"updatedAt"`
			Content   string `json:"content"`
		} `json:"files"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 1 {
		t.Fatalf("want 1 file, got %d: %s", len(out.Files), rec.Body.String())
	}
	f := out.Files[0]
	if f.Path != "notes/plan.md" || f.SizeBytes != 11 || f.UpdatedAt == "" {
		t.Fatalf("bad row: %+v", f)
	}
	if f.Content != "" {
		t.Fatalf("listing must not carry content, got %q", f.Content)
	}

	// Unknown bot → 404.
	rec = httptest.NewRecorder()
	srv.handleListBotFiles(rec, authedJSONReq(http.MethodGet, "/api/bots/missing/files", nil, "u-1"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown bot: want 404, got %d", rec.Code)
	}
}

func TestBotMemoriesEndpoints(t *testing.T) {
	bots := newFakeBots()
	mems := newFakeBotMemories()
	bot, err := bots.Create(context.Background(), storage.Bot{UserID: "u-1", Name: "Scout"})
	if err != nil {
		t.Fatal(err)
	}
	added, err := mems.Add(context.Background(), storage.BotMemory{BotID: bot.ID, UserID: "u-1", Content: "likes brevity"})
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{deps: ServerDeps{Bots: bots, BotMemories: mems}}

	// Canonical list endpoint.
	rec := httptest.NewRecorder()
	req := authedJSONReq(http.MethodGet, "/api/bots/x/memories", nil, "u-1")
	req.SetPathValue("id", bot.ID)
	srv.handleListBotMemories(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Memories []botMemoryDTO `json:"memories"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Memories) != 1 || out.Memories[0].ID != added.ID || out.Memories[0].Content != "likes brevity" {
		t.Fatalf("bad memories: %+v", out.Memories)
	}

	// Delete unknown memory → 404; known → 204 and gone.
	rec = httptest.NewRecorder()
	srv.handleDeleteBotMemory(rec, authedJSONReq(http.MethodDelete, "/api/bots/"+bot.ID+"/memories/missing", nil, "u-1"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown memory: want 404, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	req = authedJSONReq(http.MethodDelete, "/api/bots/x/memories/y", nil, "u-1")
	req.SetPathValue("id", bot.ID)
	req.SetPathValue("memoryId", added.ID)
	srv.handleDeleteBotMemory(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: want 204, got %d: %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	req = authedJSONReq(http.MethodGet, "/api/bots/x/memories", nil, "u-1")
	req.SetPathValue("id", bot.ID)
	srv.handleListBotMemories(rec, req)
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Memories) != 0 {
		t.Fatalf("memory not deleted: %+v", out.Memories)
	}

	// Unknown bot on both endpoints → 404.
	for _, tc := range []struct {
		name   string
		method string
		path   string
	}{{"files", http.MethodGet, "/api/bots/missing/memories"}, {"delete", http.MethodDelete, "/api/bots/missing/memories/x"}} {
		rec = httptest.NewRecorder()
		if tc.method == http.MethodGet {
			srv.handleListBotMemories(rec, authedJSONReq(tc.method, tc.path, nil, "u-1"))
		} else {
			srv.handleDeleteBotMemory(rec, authedJSONReq(tc.method, tc.path, nil, "u-1"))
		}
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: want 404, got %d", tc.name, rec.Code)
		}
	}
}

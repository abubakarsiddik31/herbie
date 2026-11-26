import { useCallback, useRef, useState } from "react";
import { BASE, ApiError, apiFetch, tryRefresh } from "@/lib/api";
import { parseSSE } from "@/lib/sse";
import { useAuth } from "@/stores/auth";
import type { ChatMessage, PendingApproval, Source } from "@/lib/types";

export type RunStatus = "idle" | "running" | "error";

// waitForRunToSettle polls the in-process run registry so a reconnecting
// client refetches only after the server has stopped writing the turn.
async function waitForRunToSettle(conversationId: string): Promise<void> {
  for (let i = 0; i < 12; i++) {
    try {
      const status = await apiFetch<{ active: boolean }>(`/api/conversations/${conversationId}/run`);
      if (!status.active) return;
    } catch {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
}

export interface ContextBreakdown {
  systemTokens: number;
  memoryTokens: number;
  instructionsTokens: number;
  historyTokens: number;
  promptTokens: number;
  attachmentTokens: number;
  totalTokens: number;
  exact: boolean;
  model: string;
}

export interface ContextStatus {
  estimatedTokens: number;
  thresholdTokens: number;
  keepRecent?: number;
  compacted?: boolean;
  breakdown?: ContextBreakdown;
  exact?: boolean;
}

interface DonePayload {
  messageId: string;
  promptMessageId?: string;
  inputTokens: number;
  outputTokens: number;
  requests: number;
  costUsd: number;
  model?: string;
  contextTokens?: number;
  thresholdTokens?: number;
  keepRecent?: number;
}
interface MetaPayload {
  type: string;
  inputTokens?: number;
  outputTokens?: number;
  name?: string;
  ok?: boolean;
  estimatedTokens?: number;
  thresholdTokens?: number;
  keepRecent?: number;
  breakdown?: ContextBreakdown;
  exact?: boolean;
  requested?: string;
  used?: string;
}
interface SourcesPayload { sources: Source[] }

// Model-facing tool names become human phrases in the run trace.
const toolLabels: Record<string, string> = {
  search_documents: "searching documents",
  list_documents: "listing documents",
  read_document: "reading document",
};

function toolLabel(name?: string): string {
  return (name && toolLabels[name]) || name || "tool";
}

export interface ApprovalDecision { callId: string; approved: boolean; reason?: string }

export function useChat(onDone?: () => void) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [trace, setTrace] = useState<string[]>([]);
  const [pending, setPending] = useState<PendingApproval[]>([]);
  const [status, setStatus] = useState<RunStatus>("idle");
  const [contextStatus, setContextStatus] = useState<ContextStatus | null>(null);
  const [canRetry, setCanRetry] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  const stop = useCallback(() => abortRef.current?.abort(), []);

  const reset = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setMessages([]);
    setTrace([]);
    setPending([]);
    setContextStatus(null);
    setStatus("idle");
    lastSendRef.current = null;
    setCanRetry(false);
  }, []);

  // consumeStream drains a chat SSE response (send-message or approvals),
  // feeding the assistant placeholder. Resolves when the stream ends.
  const consumeStream = useCallback(async (res: Response, assistantId: string) => {
    let received = false;
    let hadError = false;
    for await (const frame of parseSSE(res)) {
      const payload = JSON.parse(frame.data) as any; // backend SSE payloads are untyped on the wire
      if (frame.event === "delta") {
        received = true;
        setMessages((m) => m.map((msg) => msg.id === assistantId ? { ...msg, content: msg.content + payload.text } : msg));
      } else if (frame.event === "meta") {
        const meta = payload as MetaPayload;
        if (meta.type === "model_end") {
          setTrace((t) => [...t, `model call · ${meta.inputTokens ?? 0} in / ${meta.outputTokens ?? 0} out`]);
        } else if (meta.type === "context") {
          setContextStatus({
            estimatedTokens: meta.estimatedTokens ?? 0,
            thresholdTokens: meta.thresholdTokens ?? 40000,
            keepRecent: meta.keepRecent ?? 10,
            compacted: false,
            breakdown: meta.breakdown,
            exact: meta.exact ?? false,
          });
        } else if (meta.type === "compacted") {
          setTrace((t) => [...t, "earlier history summarized"]);
          setContextStatus((prev) => ({
            estimatedTokens: prev?.estimatedTokens ?? 0,
            thresholdTokens: meta.thresholdTokens ?? prev?.thresholdTokens ?? 40000,
            keepRecent: meta.keepRecent ?? prev?.keepRecent ?? 10,
            compacted: true,
            breakdown: prev?.breakdown,
            exact: prev?.exact ?? false,
          }));
        } else if (meta.type === "tool_start") {
          setTrace((t) => [...t, `${toolLabel(meta.name)}…`]);
        } else if (meta.type === "tool_end") {
          setTrace((t) => [...t, `${meta.name} ${meta.ok === false ? "failed" : "finished"}`]);
        } else if (meta.type === "fallback") {
          setTrace((t) => [...t, `fell back to ${meta.used ?? "another model"}`]);
        }
      } else if (frame.event === "sources") {
        const { sources } = payload as SourcesPayload;
        setMessages((m) => m.map((msg) => msg.id === assistantId ? { ...msg, sources } : msg));
      } else if (frame.event === "approval_request") {
        setPending((payload.calls ?? []) as PendingApproval[]);
      } else if (frame.event === "done") {
        const done = payload as DonePayload;
        setPending([]);
        if (done.contextTokens != null) {
          setContextStatus((prev) => ({
            estimatedTokens: done.contextTokens ?? (done.inputTokens + done.outputTokens),
            thresholdTokens: done.thresholdTokens ?? prev?.thresholdTokens ?? 40000,
            keepRecent: done.keepRecent ?? prev?.keepRecent ?? 10,
            compacted: prev?.compacted ?? false,
            breakdown: prev?.breakdown
              ? {
                  ...prev.breakdown,
                  totalTokens: done.contextTokens ?? prev.breakdown.totalTokens,
                  exact: false,
                }
              : undefined,
            exact: false,
          }));
        }
        setMessages((m) => m.map((msg) => msg.id === assistantId
          ? {
              ...msg,
              id: done.messageId || msg.id,
              streaming: false,
              usage: done.model
                ? { inputTokens: done.inputTokens, outputTokens: done.outputTokens, costUsd: done.costUsd, model: done.model }
                : msg.usage,
            }
          : msg));
        // Remap the optimistic prompt id to its persisted row before the
        // detail refetch lands, so an immediate edit/delete/activate targets
        // the row the server actually stored (edited messages get a new
        // sibling id; regenerate reports the existing user row unchanged).
        if (done.promptMessageId) {
          setMessages((m) => {
            const userIdx = m.map((msg) => msg.role).lastIndexOf("user");
            if (userIdx < 0 || m[userIdx].id === done.promptMessageId) return m;
            return m.map((msg, i) => i === userIdx ? { ...msg, id: done.promptMessageId! } : msg);
          });
        }
      } else if (frame.event === "error") {
        hadError = true;
        setMessages((m) => m.map((msg) => msg.id === assistantId
          ? { ...msg, streaming: false, error: payload.message, truncated: received } : msg));
      }
    }
    setMessages((m) => m.map((msg) => msg.id === assistantId ? { ...msg, streaming: false } : msg));
    setStatus(hadError ? "error" : "idle");
    onDone?.();
  }, [onDone]);

  // streamPOST issues one authenticated chat-stream request with 401 retry.
  const streamPOST = useCallback(async (path: string, body: unknown, controller: AbortController) => {
    const doSend = () => {
      const token = useAuth.getState().accessToken;
      return fetch(`${BASE}${path}`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: token ? `Bearer ${token}` : "" },
        body: JSON.stringify(body),
        credentials: "include",
        signal: controller.signal,
      });
    };
    let res = await doSend();
    if (res.status === 401 && (await tryRefresh())) {
      res = await doSend();
    }
    if (!res.ok || !res.body) {
      if (res.status === 401) useAuth.getState().clear();
      // Surface API errors (e.g. 409 approval_pending) with their message.
      let message = `stream failed (${res.status})`;
      let code = "error";
      try {
        const errBody = (await res.json()) as { error?: { code?: string; message: string } };
        if (errBody.error?.message) message = errBody.error.message;
        if (errBody.error?.code) code = errBody.error.code;
      } catch { /* non-JSON error body */ }
      throw new ApiError(res.status, code, message);
    }
    return res;
  }, []);

  // start drives one streaming request to completion and settles the
  // assistant placeholder — shared by send, edit, and regenerate.
  const start = useCallback(async (path: string, body: unknown, controller: AbortController, assistantId: string) => {
    try {
      const res = await streamPOST(path, body, controller);
      await consumeStream(res, assistantId);
    } catch (err) {
      if ((err as Error).name === "AbortError") {
        // Only settle the run if this controller is still the active one — a
        // reset()/conversation switch has already cleared the thread otherwise.
        if (abortRef.current === controller) {
          setMessages((m) => m.map((msg) => msg.id === assistantId ? { ...msg, streaming: false, truncated: true } : msg));
          setStatus("idle");
        }
      } else {
        const conversationId = path.match(/\/conversations\/([^/]+)/)?.[1];
        if (err instanceof ApiError && err.code === "run_active" && conversationId) {
          await waitForRunToSettle(conversationId);
          if (abortRef.current === controller) {
            setMessages((m) => m.map((msg) => msg.id === assistantId ? { ...msg, streaming: false } : msg));
            setStatus("idle");
          }
          onDone?.();
          return;
        }
        if (abortRef.current === controller) {
          setMessages((m) => m.map((msg) => msg.id === assistantId
            ? { ...msg, streaming: false, error: (err as Error).message }
            : msg));
          setStatus("error");
        }
        // A duplicate is already stored; the send path handles its refetch.
        if (!(err instanceof ApiError && err.code === "duplicate_send")) {
          if (conversationId) {
            void waitForRunToSettle(conversationId).then(() => onDone?.());
          } else {
            onDone?.();
          }
        }
        throw err;
      }
    }
  }, [consumeStream, streamPOST]);

  const lastSendRef = useRef<{
    content: string;
    conversationId: string;
    images?: { mediaType: string; data: string }[];
    replyToId?: string;
    quoteText?: string;
    clientMessageId: string;
  } | null>(null);

  const send = useCallback(async (
    content: string,
    conversationId: string,
    images?: { mediaType: string; data: string }[],
    replyToId?: string,
    quoteText?: string,
    clientMessageId?: string,
  ) => {
    const cleanContent = content.replace(/\0/g, "");
    const idempotencyKey = clientMessageId ?? crypto.randomUUID();
    setCanRetry(false);
    lastSendRef.current = {
      content: cleanContent, conversationId, images, replyToId, quoteText,
      clientMessageId: idempotencyKey,
    };
    const controller = new AbortController();
    abortRef.current = controller;
    const userMsg: ChatMessage = {
      id: crypto.randomUUID(), role: "user", content: cleanContent, truncated: false, createdAt: new Date().toISOString(),
      images: images?.map((img) => ({ mediaType: img.mediaType, dataUrl: `data:${img.mediaType};base64,${img.data}` })),
      replyToId,
      quoteText,
    };
    const assistantId = crypto.randomUUID();
    setMessages((m) => [
      ...m, userMsg,
      { id: assistantId, role: "assistant", content: "", truncated: false, createdAt: new Date().toISOString(), streaming: true },
    ]);
    setTrace([]);
    setPending([]);
    setStatus("running");
    const body: Record<string, unknown> = { content: cleanContent, clientMessageId: idempotencyKey };
    if (images?.length) body.images = images;
    if (replyToId) body.replyToId = replyToId;
    try {
      await start(`/api/conversations/${conversationId}/messages`, body, controller, assistantId);
    } catch (err) {
      if (err instanceof ApiError && err.code === "duplicate_send") {
        // The retry's prompt is already stored; drop the optimistic rows and
        // let the refetch show the durable transcript.
        lastSendRef.current = null;
        setCanRetry(false);
        setMessages((m) => m.filter((msg) => msg.id !== assistantId && msg.id !== userMsg.id));
        setStatus("idle");
        onDone?.();
        return;
      }
      lastSendRef.current = null;
      setCanRetry(true);
      throw err;
    }
  }, [start, onDone]);

  const retrySend = useCallback(async () => {
    const last = lastSendRef.current;
    if (!last) return;
    await send(last.content, last.conversationId, last.images, last.replyToId, last.quoteText, last.clientMessageId);
  }, [send]);

  // edit rewrites one user message and re-runs the conversation from it:
  // the local thread truncates to the edited message, then a fresh
  // assistant placeholder streams the new answer.
  const edit = useCallback(async (conversationId: string, msgId: string, content: string) => {
    const cleanContent = content.replace(/\0/g, "");
    const controller = new AbortController();
    abortRef.current = controller;
    const assistantId = crypto.randomUUID();
    setMessages((m) => {
      const idx = m.findIndex((msg) => msg.id === msgId);
      if (idx < 0) return m;
      const kept = m.slice(0, idx + 1).map((msg) => msg.id === msgId ? { ...msg, content: cleanContent } : msg);
      return [
        ...kept,
        { id: assistantId, role: "assistant", content: "", truncated: false, createdAt: new Date().toISOString(), streaming: true },
      ];
    });
    setTrace([]);
    setPending([]);
    setStatus("running");
    await start(`/api/conversations/${conversationId}/messages/${msgId}/edit`, {
      content: cleanContent,
      clientMessageId: crypto.randomUUID(),
    }, controller, assistantId);
  }, [start]);

  // regenerate replaces the last assistant answer: trailing non-user
  // messages drop locally and the last turn streams again.
  const regenerate = useCallback(async (conversationId: string, model?: string) => {
    const controller = new AbortController();
    abortRef.current = controller;
    const assistantId = crypto.randomUUID();
    setMessages((m) => {
      const lastUser = m.map((msg) => msg.role).lastIndexOf("user");
      if (lastUser < 0) return m;
      return [
        ...m.slice(0, lastUser + 1),
        { id: assistantId, role: "assistant", content: "", truncated: false, createdAt: new Date().toISOString(), streaming: true },
      ];
    });
    setTrace([]);
    setPending([]);
    setStatus("running");
    await start(`/api/conversations/${conversationId}/regenerate`, model ? { model } : {}, controller, assistantId);
  }, [start]);

  // continueRun resumes an interrupted answer as a new assistant branch
  // under the same user row; the partial answer stays off-path.
  const continueRun = useCallback(async (conversationId: string) => {
    const controller = new AbortController();
    abortRef.current = controller;
    const assistantId = crypto.randomUUID();
    setMessages((m) => {
      const lastUser = m.map((msg) => msg.role).lastIndexOf("user");
      if (lastUser < 0) return m;
      return [
        ...m.slice(0, lastUser + 1),
        { id: assistantId, role: "assistant", content: "", truncated: false, createdAt: new Date().toISOString(), streaming: true },
      ];
    });
    setTrace([]);
    setPending([]);
    setStatus("running");
    await start(`/api/conversations/${conversationId}/continue`, {}, controller, assistantId);
  }, [start]);

  const resolve = useCallback(async (conversationId: string, decisions: ApprovalDecision[]) => {
    const controller = new AbortController();
    abortRef.current = controller;
    const assistantId = crypto.randomUUID();
    setMessages((m) => [
      ...m,
      { id: assistantId, role: "assistant", content: "", truncated: false, createdAt: new Date().toISOString(), streaming: true },
    ]);
    setTrace([]);
    setStatus("running");
    await start(`/api/conversations/${conversationId}/approvals`, { decisions }, controller, assistantId);
  }, [start]);

  return {
    messages, setMessages, status, send, retrySend, canRetry, edit, regenerate,
    continueRun, resolve, stop, reset, trace, pending, contextStatus, setContextStatus,
  };
}

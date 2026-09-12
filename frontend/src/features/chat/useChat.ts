import { useCallback, useRef, useState } from "react";
import { BASE, ApiError, tryRefresh } from "@/lib/api";
import { parseSSE } from "@/lib/sse";
import { useAuth } from "@/stores/auth";
import type { ChatMessage, PendingApproval, Source } from "@/lib/types";

export type RunStatus = "idle" | "running" | "error";

interface DonePayload { messageId: string; inputTokens: number; outputTokens: number; requests: number; costUsd: number; model?: string }
interface MetaPayload { type: string; inputTokens?: number; outputTokens?: number; name?: string; ok?: boolean }
interface SourcesPayload { sources: Source[] }

// Model-facing tool names become human phrases in the run trace.
const toolLabels: Record<string, string> = {
  search_documents: "searching documents",
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
  const abortRef = useRef<AbortController | null>(null);

  const stop = useCallback(() => abortRef.current?.abort(), []);

  const reset = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setMessages([]);
    setTrace([]);
    setPending([]);
    setStatus("idle");
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
        } else if (meta.type === "tool_start") {
          setTrace((t) => [...t, `${toolLabel(meta.name)}…`]);
        } else if (meta.type === "tool_end") {
          setTrace((t) => [...t, `${meta.name} ${meta.ok === false ? "failed" : "finished"}`]);
        }
      } else if (frame.event === "sources") {
        const { sources } = payload as SourcesPayload;
        setMessages((m) => m.map((msg) => msg.id === assistantId ? { ...msg, sources } : msg));
      } else if (frame.event === "approval_request") {
        setPending((payload.calls ?? []) as PendingApproval[]);
      } else if (frame.event === "done") {
        const done = payload as DonePayload;
        setPending([]);
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
      try {
        const errBody = (await res.json()) as { error?: { message: string } };
        if (errBody.error?.message) message = errBody.error.message;
      } catch { /* non-JSON error body */ }
      throw new ApiError(res.status, "error", message);
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
        if (abortRef.current === controller) setStatus("error");
        throw err;
      }
    }
  }, [consumeStream, streamPOST]);

  const send = useCallback(async (content: string, conversationId: string, images?: { mediaType: string; data: string }[]) => {
    const controller = new AbortController();
    abortRef.current = controller;
    const userMsg: ChatMessage = {
      id: crypto.randomUUID(), role: "user", content, truncated: false, createdAt: new Date().toISOString(),
      images: images?.map((img) => ({ mediaType: img.mediaType, dataUrl: `data:${img.mediaType};base64,${img.data}` })),
    };
    const assistantId = crypto.randomUUID();
    setMessages((m) => [
      ...m, userMsg,
      { id: assistantId, role: "assistant", content: "", truncated: false, createdAt: new Date().toISOString(), streaming: true },
    ]);
    setTrace([]);
    setPending([]);
    setStatus("running");
    await start(`/api/conversations/${conversationId}/messages`, images?.length ? { content, images } : { content }, controller, assistantId);
  }, [start]);

  // edit rewrites one user message and re-runs the conversation from it:
  // the local thread truncates to the edited message, then a fresh
  // assistant placeholder streams the new answer.
  const edit = useCallback(async (conversationId: string, msgId: string, content: string) => {
    const controller = new AbortController();
    abortRef.current = controller;
    const assistantId = crypto.randomUUID();
    setMessages((m) => {
      const idx = m.findIndex((msg) => msg.id === msgId);
      if (idx < 0) return m;
      const kept = m.slice(0, idx + 1).map((msg) => msg.id === msgId ? { ...msg, content } : msg);
      return [
        ...kept,
        { id: assistantId, role: "assistant", content: "", truncated: false, createdAt: new Date().toISOString(), streaming: true },
      ];
    });
    setTrace([]);
    setPending([]);
    setStatus("running");
    await start(`/api/conversations/${conversationId}/messages/${msgId}/edit`, { content }, controller, assistantId);
  }, [start]);

  // regenerate replaces the last assistant answer: trailing non-user
  // messages drop locally and the last turn streams again.
  const regenerate = useCallback(async (conversationId: string) => {
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
    await start(`/api/conversations/${conversationId}/regenerate`, {}, controller, assistantId);
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

  return { messages, setMessages, status, send, edit, regenerate, resolve, stop, reset, trace, pending };
}

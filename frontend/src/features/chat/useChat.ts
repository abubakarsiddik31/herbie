import { useCallback, useRef, useState } from "react";
import { BASE, ApiError, tryRefresh } from "@/lib/api";
import { parseSSE } from "@/lib/sse";
import { useAuth } from "@/stores/auth";
import type { ChatMessage, PendingApproval } from "@/lib/types";

export type RunStatus = "idle" | "running" | "error";

interface DonePayload { messageId: string; inputTokens: number; outputTokens: number; requests: number; costUsd: number }
interface MetaPayload { type: string; inputTokens?: number; outputTokens?: number; name?: string; ok?: boolean }

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
          setTrace((t) => [...t, `calling ${meta.name}…`]);
        } else if (meta.type === "tool_end") {
          setTrace((t) => [...t, `${meta.name} ${meta.ok === false ? "failed" : "finished"}`]);
        }
      } else if (frame.event === "approval_request") {
        setPending((payload.calls ?? []) as PendingApproval[]);
      } else if (frame.event === "done") {
        const done = payload as DonePayload;
        setPending([]);
        setMessages((m) => m.map((msg) => msg.id === assistantId
          ? { ...msg, id: done.messageId || msg.id, streaming: false } : msg));
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

  const send = useCallback(async (content: string, conversationId: string) => {
    const controller = new AbortController();
    abortRef.current = controller;
    const userMsg: ChatMessage = { id: crypto.randomUUID(), role: "user", content, truncated: false, createdAt: new Date().toISOString() };
    const assistantId = crypto.randomUUID();
    setMessages((m) => [
      ...m, userMsg,
      { id: assistantId, role: "assistant", content: "", truncated: false, createdAt: new Date().toISOString(), streaming: true },
    ]);
    setTrace([]);
    setPending([]);
    setStatus("running");
    try {
      const res = await streamPOST(`/api/conversations/${conversationId}/messages`, { content }, controller);
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
    try {
      const res = await streamPOST(`/api/conversations/${conversationId}/approvals`, { decisions }, controller);
      await consumeStream(res, assistantId);
    } catch (err) {
      if ((err as Error).name === "AbortError") {
        if (abortRef.current === controller) setStatus("idle");
      } else {
        if (abortRef.current === controller) setStatus("error");
        throw err;
      }
    }
  }, [consumeStream, streamPOST]);

  return { messages, setMessages, status, send, resolve, stop, reset, trace, pending };
}

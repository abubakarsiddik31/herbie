import { useCallback, useRef, useState } from "react";
import { BASE, ApiError } from "@/lib/api";
import { parseSSE } from "@/lib/sse";
import { useAuth } from "@/stores/auth";
import type { ChatMessage } from "@/lib/types";

export type RunStatus = "idle" | "running" | "error";

interface DonePayload { messageId: string; inputTokens: number; outputTokens: number; requests: number; costUsd: number }
interface MetaPayload { type: string; inputTokens?: number; outputTokens?: number }

export function useChat(onDone?: () => void) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [trace, setTrace] = useState<string[]>([]);
  const [status, setStatus] = useState<RunStatus>("idle");
  const abortRef = useRef<AbortController | null>(null);

  const stop = useCallback(() => abortRef.current?.abort(), []);

  const reset = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setMessages([]);
    setTrace([]);
    setStatus("idle");
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
    setStatus("running");
    let received = false;
    let hadError = false;
    try {
      const token = useAuth.getState().accessToken;
      const res = await fetch(`${BASE}/api/conversations/${conversationId}/messages`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
        body: JSON.stringify({ content }),
        credentials: "include",
        signal: controller.signal,
      });
      if (!res.ok || !res.body) {
        if (res.status === 401) useAuth.getState().clear();
        throw new ApiError(res.status, "error", `stream failed (${res.status})`);
      }
      let modelCalls = 0;
      for await (const frame of parseSSE(res)) {
        const payload = JSON.parse(frame.data) as any; // backend SSE payloads are untyped on the wire
        if (frame.event === "delta") {
          received = true;
          setMessages((m) => m.map((msg) => msg.id === assistantId ? { ...msg, content: msg.content + payload.text } : msg));
        } else if (frame.event === "meta") {
          const meta = payload as MetaPayload;
          if (meta.type === "model_end") {
            modelCalls += 1;
            setTrace((t) => [...t, `model call #${modelCalls} · ${meta.inputTokens ?? 0} in / ${meta.outputTokens ?? 0} out`]);
          }
        } else if (frame.event === "done") {
          const done = payload as DonePayload;
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
  }, [onDone]);

  return { messages, setMessages, status, send, stop, reset, trace };
}

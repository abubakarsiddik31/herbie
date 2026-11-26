import { act } from "react";
import { renderHook } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { useChat } from "./useChat";
import { useAuth } from "@/stores/auth";

const sseBody = (frames: string[]) =>
  new ReadableStream({
    start(controller) {
      for (const frame of frames) controller.enqueue(new TextEncoder().encode(frame));
      controller.close();
    },
  });

const server = setupServer(
  http.post("*/api/auth/refresh", () => HttpResponse.json({ accessToken: "fresh" })),
  http.post("*/api/conversations/c1/messages", ({ request }) => {
    if (request.headers.get("Authorization") !== "Bearer fresh") {
      return HttpResponse.json({ error: { code: "unauthorized", message: "expired" } }, { status: 401 });
    }
    return new HttpResponse(
      sseBody([
        'event: delta\ndata: {"text":"pong"}\n\n',
        'event: done\ndata: {"messageId":"m1","inputTokens":1,"outputTokens":2,"requests":1,"costUsd":0.00001}\n\n',
      ]),
      { headers: { "Content-Type": "text/event-stream" } },
    );
  }),
);

beforeAll(() => {
  server.listen({ onUnhandledRequest: "error" });
});
beforeEach(() => {
  useAuth.setState({ user: { id: "u", email: "a@b.co" }, accessToken: "stale" });
});
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

describe("useChat regenerate", () => {
  it("sends the model override for one regenerated branch", async () => {
    let body: unknown = null;
    server.use(
      http.post("*/api/conversations/c1/regenerate", async ({ request }) => {
        body = await request.json();
        return new HttpResponse(
          sseBody([
            'event: delta\ndata: {"text":"again"}\n\n',
            'event: done\ndata: {"messageId":"m2","inputTokens":1,"outputTokens":1,"requests":1,"costUsd":0.00001,"model":"gemini-3.5-pro"}\n\n',
          ]),
          { headers: { "Content-Type": "text/event-stream" } },
        );
      }),
    );
    const { result } = renderHook(() => useChat());
    act(() => {
      result.current.setMessages([
        { id: "u1", role: "user", content: "hi", truncated: false, createdAt: "2026-01-01T00:00:00Z" },
      ]);
    });
    await act(async () => {
      await result.current.regenerate("c1", "gemini-3.5-pro");
    });
    expect(body).toEqual({ model: "gemini-3.5-pro" });
    expect(result.current.messages.find((m) => m.role === "assistant")?.content).toBe("again");
  });
});

describe("useChat send", () => {
  it("refreshes once on 401 and retries the SSE stream with the new token", async () => {
    const { result } = renderHook(() => useChat());
    await act(async () => {
      await result.current.send("hi", "c1");
    });
    const assistant = result.current.messages.find((m) => m.role === "assistant");
    expect(assistant?.content).toBe("pong");
    expect(useAuth.getState().accessToken).toBe("fresh");
  });

  it("records an auto-fallback in the run trace", async () => {
    server.use(
      http.post("*/api/conversations/c1/messages", () =>
        new HttpResponse(
          sseBody([
            'event: meta\ndata: {"type":"fallback","requested":"auto","used":"gemini-3.5-flash"}\n\n',
            'event: delta\ndata: {"text":"hi"}\n\n',
            'event: done\ndata: {"messageId":"m1","inputTokens":1,"outputTokens":2,"requests":1,"costUsd":0.00001,"model":"gemini-3.5-flash"}\n\n',
          ]),
          { headers: { "Content-Type": "text/event-stream" } },
        ),
      ),
    );
    const { result } = renderHook(() => useChat());
    await act(async () => {
      await result.current.send("hi", "c1");
    });
    expect(result.current.trace).toContain("fell back to gemini-3.5-flash");
  });

  it("surfaces history compaction in the run trace", async () => {
    server.use(
      http.post("*/api/conversations/c1/messages", () =>
        new HttpResponse(
          sseBody([
            'event: meta\ndata: {"type":"compacted"}\n\n',
            'event: delta\ndata: {"text":"hi"}\n\n',
            'event: done\ndata: {"messageId":"m1","inputTokens":1,"outputTokens":2,"requests":1,"costUsd":0.00001}\n\n',
          ]),
          { headers: { "Content-Type": "text/event-stream" } },
        ),
      ),
    );
    const { result } = renderHook(() => useChat());
    await act(async () => {
      await result.current.send("hi", "c1");
    });
    expect(result.current.trace).toContain("earlier history summarized");
  });

  it("waits out an active run and refetches instead of erroring", async () => {
    server.use(
      http.get("*/api/conversations/c1/run", () => HttpResponse.json({ active: false })),
      http.post("*/api/conversations/c1/messages", () =>
        HttpResponse.json(
          { error: { code: "run_active", message: "A run is in progress" } },
          { status: 409 },
        ),
      ),
    );
    const onDone = vi.fn();
    const { result } = renderHook(() => useChat(onDone));
    await act(async () => {
      await result.current.send("hi", "c1");
    });
    const assistant = result.current.messages.find((m) => m.role === "assistant");
    expect(assistant).toMatchObject({ streaming: false });
    expect(assistant?.error).toBeUndefined();
    expect(result.current.status).toBe("idle");
    expect(onDone).toHaveBeenCalled();
  });

  it("tracks context status and compaction updates from stream frames", async () => {
    server.use(
      http.post("*/api/conversations/c1/messages", () =>
        new HttpResponse(
          sseBody([
            'event: meta\ndata: {"type":"context","estimatedTokens":2400,"thresholdTokens":40000,"keepRecent":10,"exact":true,"breakdown":{"systemTokens":800,"memoryTokens":100,"instructionsTokens":50,"historyTokens":1500,"promptTokens":60,"attachmentTokens":0,"totalTokens":2400,"exact":true,"model":"gemini-3.5-flash"}}\n\n',
            'event: meta\ndata: {"type":"compacted","thresholdTokens":40000,"keepRecent":10}\n\n',
            'event: delta\ndata: {"text":"hello"}\n\n',
            'event: done\ndata: {"messageId":"m1","inputTokens":10,"outputTokens":5,"requests":1,"costUsd":0.00001,"contextTokens":2415,"thresholdTokens":40000,"keepRecent":10}\n\n',
          ]),
          { headers: { "Content-Type": "text/event-stream" } },
        ),
      ),
    );
    const { result } = renderHook(() => useChat());
    await act(async () => {
      await result.current.send("hi", "c1");
    });
    expect(result.current.contextStatus).toMatchObject({
      estimatedTokens: 2415,
      thresholdTokens: 40000,
      keepRecent: 10,
      compacted: true,
      exact: false,
      breakdown: { totalTokens: 2415, exact: false, model: "gemini-3.5-flash" },
    });
  });
});

describe("useChat continue", () => {
  it("streams a continuation and replaces trailing assistant rows", async () => {
    server.use(
      http.post("*/api/conversations/c1/continue", () =>
        new HttpResponse(
          sseBody([
            'event: delta\ndata: {"text":" more text"}\n\n',
            'event: done\ndata: {"messageId":"a2","inputTokens":1,"outputTokens":1,"requests":1,"costUsd":0}\n\n',
          ]),
          { headers: { "Content-Type": "text/event-stream" } },
        ),
      ),
    );
    const { result } = renderHook(() => useChat());
    act(() =>
      result.current.setMessages([
        { id: "u1", role: "user", content: "question", truncated: false, createdAt: "" },
        { id: "a1", role: "assistant", content: "partial", truncated: true, createdAt: "" },
      ]),
    );
    await act(async () => {
      await result.current.continueRun("c1");
    });
    const assistant = result.current.messages[result.current.messages.length - 1];
    expect(assistant.role).toBe("assistant");
    expect(assistant.content).toContain("more text");
    expect(result.current.messages.filter((m) => m.role === "user")).toHaveLength(1);
    expect(result.current.messages.filter((m) => m.role === "assistant")).toHaveLength(2);
    expect(result.current.messages[1].content).toBe("partial");
  });
});

describe("useChat idempotent sends", () => {
  it("sends a clientMessageId and treats duplicate_send as already stored", async () => {
    const captured: { body: Record<string, unknown> | null } = { body: null };
    server.use(
      http.post("*/api/conversations/c1/messages", async ({ request }) => {
        captured.body = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json(
          { error: { code: "duplicate_send", message: "already sent" } },
          { status: 409 },
        );
      }),
    );
    const onDone = vi.fn();
    const { result } = renderHook(() => useChat(onDone));
    await act(async () => {
      await result.current.send("hi", "c1");
    });
    expect(captured.body?.clientMessageId).toEqual(expect.any(String));
    expect(onDone).toHaveBeenCalled();
    expect(result.current.status).toBe("idle");
    expect(result.current.messages).toHaveLength(0);
  });
});

describe("useChat prompt id remap", () => {
  it("replaces the optimistic user id with the persisted prompt row id", async () => {
    server.use(
      http.post("*/api/conversations/c1/messages", () =>
        new HttpResponse(
          sseBody([
            'event: delta\ndata: {"text":"pong"}\n\n',
            'event: done\ndata: {"messageId":"m1","promptMessageId":"srv-user","inputTokens":1,"outputTokens":2,"requests":1,"costUsd":0.00001}\n\n',
          ]),
          { headers: { "Content-Type": "text/event-stream" } },
        ),
      ),
    );
    const { result } = renderHook(() => useChat());
    await act(async () => {
      await result.current.send("hi", "c1");
    });
    const user = result.current.messages.find((m) => m.role === "user");
    expect(user?.id).toBe("srv-user");
  });
});

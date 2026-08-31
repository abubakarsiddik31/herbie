import { act } from "react";
import { renderHook } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeEach, describe, expect, it } from "vitest";
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

beforeEach(() => {
  server.listen({ onUnhandledRequest: "error" });
  useAuth.setState({ user: { id: "u", email: "a@b.co" }, accessToken: "stale" });
});
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

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
});

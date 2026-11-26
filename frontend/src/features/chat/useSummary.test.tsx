import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from "vitest";
import { useAuth } from "@/stores/auth";
import { useClearSummary, useCompactNow, useConversationSummary, useSaveSummary } from "./useSummary";

const base = {
  summary: "earlier decisions",
  coveredMessages: 3,
  totalMessages: 8,
  thresholdTokens: 40000,
  keepRecent: 10,
};

let saved = "";
let deleted = 0;

const server = setupServer(
  http.get("*/api/conversations/c1/summary", () => HttpResponse.json(base)),
  http.put("*/api/conversations/c1/summary", async ({ request }) => {
    saved = ((await request.json()) as { summary: string }).summary;
    return new HttpResponse(null, { status: 204 });
  }),
  http.delete("*/api/conversations/c1/summary", () => {
    deleted += 1;
    return new HttpResponse(null, { status: 204 });
  }),
  http.post("*/api/conversations/c1/compact", () =>
    HttpResponse.json({ ...base, summary: "generated", coveredMessages: 5 }),
  ),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => {
  useAuth.setState({ user: { id: "u1", email: "a@b.co" }, accessToken: "tok" });
  saved = "";
  deleted = 0;
});
afterEach(() => {
  server.resetHandlers();
  useAuth.getState().clear();
});
afterAll(() => server.close());

const client = new QueryClient({
  defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
});
function wrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe("summary hooks", () => {
  it("loads, saves, compacts and clears a summary", async () => {
    const summary = renderHook(() => useConversationSummary("c1"), { wrapper });
    await waitFor(() => expect(summary.result.current.data?.summary).toBe("earlier decisions"));

    const save = renderHook(() => useSaveSummary("c1"), { wrapper });
    await act(async () => {
      await save.result.current.mutateAsync("edited");
    });
    expect(saved).toBe("edited");

    const compact = renderHook(() => useCompactNow("c1"), { wrapper });
    let generated = "";
    await act(async () => {
      generated = (await compact.result.current.mutateAsync()).summary;
    });
    expect(generated).toBe("generated");

    const clear = renderHook(() => useClearSummary("c1"), { wrapper });
    await act(async () => {
      await clear.result.current.mutateAsync();
    });
    expect(deleted).toBe(1);
  });
});

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { useCompare } from "./useCompare";

const server = setupServer(
  http.post("*/api/conversations/c1/compare", async ({ request }) => {
    const body = (await request.json()) as { content: string; models: string[] };
    return HttpResponse.json({
      promptMessageId: "p1",
      results: body.models.map((model, i) => ({
        model,
        messageId: `a${i}`,
        content: `answer ${i}`,
        inputTokens: 1,
        outputTokens: 1,
        costUsd: 0,
      })),
    });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

const client = new QueryClient({
  defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
});
function wrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe("useCompare", () => {
  it("posts the prompt and selected models", async () => {
    const { result } = renderHook(() => useCompare("c1"), { wrapper });
    let data: Awaited<ReturnType<typeof result.current.mutateAsync>> | undefined;
    await act(async () => {
      data = await result.current.mutateAsync({
        content: "compare",
        models: ["gemini-3.5-flash", "gemini-3.5-pro"],
      });
    });
    expect(data?.results).toHaveLength(2);
  });
});

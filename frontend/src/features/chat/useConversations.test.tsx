import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import type { ReactNode } from "react";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { useConversations } from "./useConversations";

let seenQueries: (string | null)[] = [];

const server = setupServer(
  http.get("*/api/conversations", ({ request }) => {
    seenQueries.push(new URL(request.url).searchParams.get("q"));
    return HttpResponse.json([]);
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  server.resetHandlers();
  seenQueries = [];
});
afterAll(() => server.close());

function wrapper() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe("useConversations", () => {
  it("lists without a query by default and filters with one", async () => {
    const { result, rerender } = renderHook(({ search }) => useConversations(search), {
      wrapper: wrapper(),
      initialProps: { search: "" },
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(seenQueries).toEqual([null]);
    rerender({ search: "moon" });
    await waitFor(() => expect(seenQueries).toEqual([null, "moon"]));
  });
});

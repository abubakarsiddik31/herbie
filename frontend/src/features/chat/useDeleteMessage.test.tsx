import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import type { ReactNode } from "react";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { useDeleteMessage } from "./useConversations";

let deleted: string[] = [];

const server = setupServer(
  http.delete("*/api/conversations/:id/messages/:messageId", ({ params }) => {
    deleted.push(`${params.id}/${params.messageId}`);
    return new HttpResponse(null, { status: 204 });
  }),
  http.get("*/api/conversations", () => HttpResponse.json([])),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  server.resetHandlers();
  deleted = [];
});
afterAll(() => server.close());

function wrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe("useDeleteMessage", () => {
  it("deletes the message and refreshes the thread", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(["conversation", "c1"], { conversation: { id: "c1" }, messages: [] });
    const invalidate = vi.spyOn(client, "invalidateQueries");
    const { result } = renderHook(() => useDeleteMessage("c1"), { wrapper: wrapper(client) });

    result.current.mutate("m-3");
    await waitFor(() => expect(deleted).toEqual(["c1/m-3"]));
    await waitFor(() =>
      expect(invalidate).toHaveBeenCalledWith({ queryKey: ["conversation", "c1"] }),
    );
  });
});

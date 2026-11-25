import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { useAuth } from "@/stores/auth";
import { useDeleteAccount, useExportAccount } from "./useAccount";

let deletedBody: unknown = null;

const server = setupServer(
  http.get("*/api/account/export", () =>
    new HttpResponse(new Uint8Array([1, 2, 3]), {
      status: 200,
      headers: { "Content-Type": "application/zip" },
    }),
  ),
  http.post("*/api/account/delete", async ({ request }) => {
    deletedBody = await request.json();
    return new HttpResponse(null, { status: 204 });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => {
  useAuth.setState({ user: { id: "u1", email: "a@b.co" }, accessToken: "tok" });
  deletedBody = null;
});
afterEach(() => {
  server.resetHandlers();
  useAuth.getState().clear();
  vi.restoreAllMocks();
});
afterAll(() => server.close());

const client = new QueryClient({
  defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
});

function wrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe("account hooks", () => {
  it("downloads the export blob", async () => {
    const create = vi.fn(() => "blob:herbie");
    const revoke = vi.fn();
    Object.defineProperty(URL, "createObjectURL", { value: create, configurable: true });
    Object.defineProperty(URL, "revokeObjectURL", { value: revoke, configurable: true });
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});

    const { result } = renderHook(() => useExportAccount(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync();
    });
    expect(create).toHaveBeenCalledTimes(1);
    expect(click).toHaveBeenCalledTimes(1);
  });

  it("posts the delete confirmation", async () => {
    const { result } = renderHook(() => useDeleteAccount(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync({ password: "pw", confirm: "DELETE" });
    });
    expect(deletedBody).toEqual({ password: "pw", confirm: "DELETE" });
  });
});

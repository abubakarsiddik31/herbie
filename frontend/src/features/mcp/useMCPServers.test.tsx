import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import type { ReactNode } from "react";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { useMCPServers, useCreateMCPServer, useToggleMCPServer, useDeleteMCPServer, useTestMCPServer } from "./useMCPServers";

let createdServerName: string | null = null;
let toggledId: string | null = null;
let deletedId: string | null = null;
let testedUrl: string | null = null;

const server = setupServer(
  http.get("*/api/mcp/servers", () =>
    HttpResponse.json({
      servers: [
        {
          id: "mcp-1",
          userId: "user-1",
          name: "sqlite_mcp",
          url: "http://localhost:3000/mcp",
          transport: "http",
          enabled: true,
          createdAt: "2026-09-19T00:00:00Z",
          updatedAt: "2026-09-19T00:00:00Z",
        },
      ],
    }),
  ),
  http.post("*/api/mcp/servers", async ({ request }) => {
    const body = (await request.json()) as { name: string; url: string };
    createdServerName = body.name;
    return HttpResponse.json(
      {
        server: {
          id: "mcp-2",
          userId: "user-1",
          name: body.name,
          url: body.url,
          transport: "http",
          enabled: true,
          createdAt: "2026-09-19T00:00:00Z",
          updatedAt: "2026-09-19T00:00:00Z",
        },
      },
      { status: 201 },
    );
  }),
  http.post("*/api/mcp/servers/:id/toggle", ({ params }) => {
    toggledId = params.id as string;
    return HttpResponse.json({ server: { id: params.id, enabled: false } });
  }),
  http.delete("*/api/mcp/servers/:id", ({ params }) => {
    deletedId = params.id as string;
    return HttpResponse.json({ ok: true });
  }),
  http.post("*/api/mcp/servers/test", async ({ request }) => {
    const body = (await request.json()) as { url: string };
    testedUrl = body.url;
    return HttpResponse.json({
      ok: true,
      count: 1,
      tools: [{ name: "query", description: "Run query", inputSchema: {} }],
    });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  server.resetHandlers();
  createdServerName = null;
  toggledId = null;
  deletedId = null;
  testedUrl = null;
});
afterAll(() => server.close());

function wrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe("useMCPServers hooks", () => {
  it("fetches registered MCP servers", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useMCPServers(), { wrapper: wrapper(client) });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.length).toBe(1);
    expect(result.current.data?.[0].name).toBe("sqlite_mcp");
  });

  it("creates a new MCP server", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useCreateMCPServer(), { wrapper: wrapper(client) });

    result.current.mutate({ name: "brave_mcp", url: "http://localhost:8000/mcp" });
    await waitFor(() => expect(createdServerName).toBe("brave_mcp"));
  });

  it("toggles and deletes an MCP server", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const toggleHook = renderHook(() => useToggleMCPServer(), { wrapper: wrapper(client) });
    const deleteHook = renderHook(() => useDeleteMCPServer(), { wrapper: wrapper(client) });

    toggleHook.result.current.mutate("mcp-1");
    await waitFor(() => expect(toggledId).toBe("mcp-1"));

    deleteHook.result.current.mutate("mcp-1");
    await waitFor(() => expect(deletedId).toBe("mcp-1"));
  });

  it("tests MCP server connectivity", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const testHook = renderHook(() => useTestMCPServer(), { wrapper: wrapper(client) });

    const res = await testHook.result.current.mutateAsync({ url: "http://localhost:9000/mcp" });
    expect(testedUrl).toBe("http://localhost:9000/mcp");
    expect(res.ok).toBe(true);
    expect(res.tools[0].name).toBe("query");
  });
});

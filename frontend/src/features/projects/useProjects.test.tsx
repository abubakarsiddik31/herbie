import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from "vitest";
import { useAuth } from "@/stores/auth";
import { useUploadProjectFile } from "./useProjects";

let capturedAuthHeader: string | null = null;
let capturedContentType: string | null = null;

const server = setupServer(
  http.post("*/api/auth/refresh", () => HttpResponse.json({ error: { code: "unauthorized", message: "no" } }, { status: 401 })),
  http.post("*/api/projects/:id/files", async ({ request }) => {
    capturedAuthHeader = request.headers.get("Authorization");
    capturedContentType = request.headers.get("Content-Type");
    if (!capturedAuthHeader || !capturedAuthHeader.startsWith("Bearer test-token")) {
      return HttpResponse.json(
        { error: { code: "unauthorized", message: "missing bearer token" } },
        { status: 401 },
      );
    }
    return HttpResponse.json(
      {
        id: "doc-1",
        filename: "test.txt",
        mime: "text/plain",
        sizeBytes: 12,
        status: "processing",
        chunkCount: 0,
        createdAt: new Date().toISOString(),
      },
      { status: 202 },
    );
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => {
  capturedAuthHeader = null;
  capturedContentType = null;
  useAuth.setState({
    user: { id: "u-1", email: "test@example.com" },
    accessToken: "test-token",
  });
});
afterEach(() => {
  server.resetHandlers();
  useAuth.getState().clear();
});
afterAll(() => server.close());

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe("useUploadProjectFile", () => {
  it("uploads a file with the Authorization bearer header", async () => {
    const { result } = renderHook(() => useUploadProjectFile("proj-1"), { wrapper });

    const file = new File(["hello world!"], "test.txt", { type: "text/plain" });
    result.current.mutate(file);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(capturedAuthHeader).toBe("Bearer test-token");
    expect(capturedContentType).toContain("multipart/form-data");
    expect(result.current.data?.filename).toBe("test.txt");
  });

  it("fails with ApiError when unauthorized", async () => {
    useAuth.setState({ user: null, accessToken: null });
    const { result } = renderHook(() => useUploadProjectFile("proj-1"), { wrapper });

    const file = new File(["hello world!"], "test.txt", { type: "text/plain" });
    result.current.mutate(file);

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.message).toBe("missing bearer token");
  });
});

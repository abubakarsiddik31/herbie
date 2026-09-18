import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api";
import { pollIntervalFor, useDocuments } from "./useDocuments";
import type { DocumentRec } from "@/lib/types";

const doc = (status: DocumentRec["status"]): DocumentRec => ({
  id: "d",
  filename: "a.md",
  mime: "text/markdown",
  sizeBytes: 10,
  status,
  chunkCount: 1,
  createdAt: new Date().toISOString(),
});

describe("pollIntervalFor", () => {
  it("polls while any document is processing", () => {
    expect(pollIntervalFor([doc("ready"), doc("processing")])).toBe(2000);
  });

  it("stops polling when all documents settle", () => {
    expect(pollIntervalFor([doc("ready"), doc("failed")])).toBe(false);
  });

  it("stops polling on empty or missing lists", () => {
    expect(pollIntervalFor([])).toBe(false);
    expect(pollIntervalFor(undefined)).toBe(false);
  });
});

const server = setupServer();

beforeAll(() => server.listen());
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe("useDocuments errors", () => {
  it("surfaces rag_disabled when the RAG stack is off", async () => {
    server.use(
      http.get("*/api/documents", () =>
        HttpResponse.json(
          { error: { code: "rag_disabled", message: "documents require RAG_ENABLED=true" } },
          { status: 503 },
        ),
      ),
    );
    const { result } = renderHook(() => useDocuments(), { wrapper });
    await waitFor(() => expect(result.current.isError).toBe(true));
    const err = result.current.error as ApiError;
    expect(err).toBeInstanceOf(ApiError);
    expect(err.code).toBe("rag_disabled");
  });
});

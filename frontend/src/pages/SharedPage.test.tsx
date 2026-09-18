import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { SharedPage } from "./SharedPage";

const thread = {
  title: "Shared thread",
  messages: [
    { id: "m1", role: "user", content: "What is this?", truncated: false, createdAt: "2026-01-01T00:00:00Z" },
    {
      id: "m2", role: "assistant", content: "An **answer**.", truncated: false,
      createdAt: "2026-01-01T00:01:00Z",
      sources: [
        { documentId: "d1", title: "doc.md", heading: "H", page: 0, score: 0.9, snippet: "proof" },
      ],
    },
  ],
};

const server = setupServer(
  http.get("*/api/shared/good", () => HttpResponse.json(thread)),
  http.get("*/api/shared/:token", () =>
    HttpResponse.json({ error: { code: "not_found", message: "gone" } }, { status: 404 }),
  ),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  cleanup();
  server.resetHandlers();
});
afterAll(() => server.close());

function renderShared(token: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[`/s/${token}`]}>
        <Routes>
          <Route path="/s/:token" element={<SharedPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("SharedPage", () => {
  it("renders the public thread with markdown and sources", async () => {
    renderShared("good");
    expect(await screen.findByText("Shared thread")).toBeInTheDocument();
    expect(await screen.findByText("What is this?")).toBeInTheDocument();
    expect(await screen.findByText("answer")).toBeInTheDocument();
    await userEvent.click(await screen.findByRole("button", { name: "Sources (1)" }));
    expect(await screen.findByText("doc.md")).toBeInTheDocument();
  });

  it("explains revoked or unknown links", async () => {
    renderShared("bogus");
    expect(await screen.findByText("Link unavailable")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open Golem" })).toHaveAttribute("href", "/");
  });
});

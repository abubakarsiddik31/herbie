import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { UsagePage } from "./UsagePage";
import { SidebarProvider } from "@/components/layout/SidebarContext";

const mockUsageData = {
  totals: [
    {
      kind: "chat",
      model: "gpt-4o",
      inputTokens: 12000,
      outputTokens: 4000,
      requests: 25,
      costUsd: 0.125,
    },
    {
      kind: "embedding",
      model: "text-embedding-3-small",
      inputTokens: 50000,
      outputTokens: 0,
      requests: 10,
      costUsd: 0.001,
    },
  ],
  daily: [
    {
      day: "2026-09-18",
      inputTokens: 30000,
      outputTokens: 2000,
      costUsd: 0.05,
    },
    {
      day: "2026-09-19",
      inputTokens: 32000,
      outputTokens: 2000,
      costUsd: 0.076,
    },
  ],
  documents: [
    {
      documentId: "doc-1",
      filename: "quarterly-report.pdf",
      inputTokens: 50000,
      costUsd: 0.001,
    },
  ],
  searches: {
    totalQueries: 15,
    webQueries: 10,
    docQueries: 5,
    byProvider: [
      { provider: "tavily", count: 8 },
      { provider: "brave", count: 2 },
      { provider: "rag", count: 5 },
    ],
    daily: [
      { day: "2026-09-18", count: 7 },
      { day: "2026-09-19", count: 8 },
    ],
    recent: [
      {
        id: "sq-1",
        query: "latest AI news",
        kind: "web_search",
        provider: "tavily",
        resultsCount: 5,
        durationMs: 150,
        createdAt: "2026-09-19T10:00:00Z",
      },
    ],
    topQueries: [
      { query: "latest AI news", count: 3 },
    ],
  },
  tools: {
    totalExecutions: 8,
    sandboxExecutions: 6,
    successCount: 8,
    failedCount: 0,
    avgDurationMs: 95,
    byTool: [
      { toolName: "code_runner", count: 6 },
      { toolName: "google_calendar", count: 2 },
    ],
  },
};

let requestedDays = 30;

const server = setupServer(
  http.get("*/api/usage/summary", ({ request }) => {
    const url = new URL(request.url);
    const d = Number(url.searchParams.get("days")) || 30;
    requestedDays = d;
    return HttpResponse.json(mockUsageData);
  }),
);

beforeAll(() => server.listen());
afterEach(() => {
  cleanup();
  server.resetHandlers();
  requestedDays = 30;
});
afterAll(() => server.close());

function renderUsagePage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <SidebarProvider>
          <UsagePage />
        </SidebarProvider>
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe("UsagePage", () => {
  it("renders KPI cards and resource consumption data", async () => {
    renderUsagePage();

    // Top title
    expect(await screen.findByText("Usage & Cost Ledger")).toBeInTheDocument();

    // Wait for KPI Cards to load from query
    expect(await screen.findByText("Total Spend")).toBeInTheDocument();
    expect(screen.getByText("Tokens Processed")).toBeInTheDocument();
    expect(screen.getByText("API Invocations")).toBeInTheDocument();
    expect(screen.getByText("Web Searches")).toBeInTheDocument();
    expect(screen.getByText("Avg. Cost / Request")).toBeInTheDocument();

    // Side panel sections
    expect(screen.getByText("Spend by Category")).toBeInTheDocument();
    expect(screen.getByText("Top Models")).toBeInTheDocument();
    expect(screen.getByText("Web Search Overview")).toBeInTheDocument();
    expect(screen.getByText("Document Indexing")).toBeInTheDocument();
    expect(screen.getByText("quarterly-report.pdf")).toBeInTheDocument();
  });

  it("switches time range filter pills (7d, 30d, 90d)", async () => {
    const user = userEvent.setup();
    renderUsagePage();

    await screen.findByText("Usage & Cost Ledger");

    const btn7d = screen.getByRole("button", { name: "7d" });
    await user.click(btn7d);

    expect(requestedDays).toBe(7);
  });

  it("toggles between Cost and Tokens metric in trend chart", async () => {
    const user = userEvent.setup();
    renderUsagePage();

    await screen.findByText("Daily Activity & Spend");

    const tokensBtn = screen.getByRole("button", { name: "Tokens" });
    await user.click(tokensBtn);

    expect(screen.getByText("Daily token throughput")).toBeInTheDocument();

    const costBtn = screen.getByRole("button", { name: "Cost ($)" });
    await user.click(costBtn);

    expect(screen.getByText("Daily expenditure in USD")).toBeInTheDocument();
  });

  it("filters ledger rows using search input", async () => {
    const user = userEvent.setup();
    renderUsagePage();

    await screen.findByText("Activity Ledger");
    const gptItems = await screen.findAllByText("gpt-4o");
    expect(gptItems.length).toBeGreaterThan(0);

    const input = screen.getByPlaceholderText("Filter models or kind…");
    await user.type(input, "embedding");

    // "embedding" matches text-embedding-3-small in the table
    const embeddingItems = screen.getAllByText("text-embedding-3-small");
    expect(embeddingItems.length).toBeGreaterThan(0);
    // In the ledger table, gpt-4o row should be filtered out
    expect(screen.queryByRole("cell", { name: /gpt-4o/i })).not.toBeInTheDocument();
  });

  it("renders empty state when there is zero usage", async () => {
    server.use(
      http.get("*/api/usage/summary", () =>
        HttpResponse.json({ totals: [], daily: [], documents: [] })
      )
    );

    renderUsagePage();

    expect(await screen.findByText("No usage recorded yet")).toBeInTheDocument();
    expect(screen.getByText("Start a chat")).toBeInTheDocument();
  });

  it("renders Web Search Overview with numeric metrics and no query details or rag queries", async () => {
    renderUsagePage();

    expect(await screen.findByText("Web Search Overview")).toBeInTheDocument();
    expect(screen.getAllByText("10 searches").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("Total Searches")).toBeInTheDocument();
    expect(screen.getByText("Avg Results")).toBeInTheDocument();
    expect(screen.getAllByText("Avg Latency").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("Searches / Day")).toBeInTheDocument();

    // Ensures details, rag queries, and internal provider 'wigolo' are not shown
    expect(screen.queryByText(/5 doc/i)).not.toBeInTheDocument();
    expect(screen.queryByText('"latest AI news"')).not.toBeInTheDocument();
    expect(screen.queryByText("Recent AI Search Queries")).not.toBeInTheDocument();
    expect(screen.queryByText("Frequent Queries")).not.toBeInTheDocument();
    expect(screen.queryByText("wigolo")).not.toBeInTheDocument();
  });

  it("renders Tools & Sandbox Overview with numeric metrics and friendly tool names", async () => {
    renderUsagePage();

    expect(await screen.findByText("Tools & Sandbox Overview")).toBeInTheDocument();
    expect(screen.getByText("8 runs")).toBeInTheDocument();
    expect(screen.getByText("Total Runs")).toBeInTheDocument();
    expect(screen.getAllByText("Code Sandbox").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("Success Rate")).toBeInTheDocument();
    expect(screen.getByText("100%")).toBeInTheDocument();
    expect(screen.getByText("95ms")).toBeInTheDocument();
    expect(screen.getByText("Google Calendar")).toBeInTheDocument();
  });
});

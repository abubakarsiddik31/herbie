import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { useAuth } from "@/stores/auth";
import { SummaryDialog } from "./SummaryDialog";

const server = setupServer(
  http.get("*/api/conversations/c1/summary", () =>
    HttpResponse.json({
      summary: "earlier decisions",
      coveredMessages: 3,
      totalMessages: 8,
      thresholdTokens: 40000,
      keepRecent: 10,
    }),
  ),
  http.post("*/api/conversations/c1/compact", () =>
    HttpResponse.json({
      summary: "generated summary",
      coveredMessages: 5,
      totalMessages: 8,
      thresholdTokens: 40000,
      keepRecent: 10,
    }),
  ),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function renderDialog() {
  useAuth.setState({ user: { id: "u1", email: "a@b.co" }, accessToken: "tok" });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <SummaryDialog open={true} onOpenChange={() => {}} conversationId="c1" />
    </QueryClientProvider>,
  );
}

describe("SummaryDialog", () => {
  it("shows the stored summary and replaces it after summarizing now", async () => {
    const user = userEvent.setup();
    renderDialog();
    const textarea = await screen.findByRole("textbox");
    expect(textarea).toHaveValue("earlier decisions");
    await user.click(screen.getByRole("button", { name: "Summarize now" }));
    expect(await screen.findByDisplayValue("generated summary")).toBeInTheDocument();
    expect(screen.getByText(/Covered:/)).toBeInTheDocument();
  });
});

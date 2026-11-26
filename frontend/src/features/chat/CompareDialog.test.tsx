import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import type { ModelsResponse } from "@/lib/types";
import { useAuth } from "@/stores/auth";
import { CompareDialog } from "./CompareDialog";

const models: ModelsResponse = {
  default: "gemini-3.5-flash",
  models: [
    { id: "gemini-3.5-flash", label: "Gemini 3.5 Flash", provider: "gemini" },
    { id: "gemini-3.5-pro", label: "Gemini 3.5 Pro", provider: "gemini" },
  ],
};

let activated = "";

const server = setupServer(
  http.post("*/api/conversations/c1/compare", () =>
    HttpResponse.json({
      promptMessageId: "p1",
      results: [
        { model: "gemini-3.5-flash", messageId: "a1", content: "first answer", inputTokens: 1, outputTokens: 1, costUsd: 0 },
        { model: "gemini-3.5-pro", messageId: "a2", content: "second answer", inputTokens: 1, outputTokens: 1, costUsd: 0 },
      ],
    }),
  ),
  http.post("*/api/conversations/c1/messages/:id/activate", ({ params }) => {
    activated = String(params.id);
    return new HttpResponse(null, { status: 204 });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  activated = "";
  server.resetHandlers();
});
afterAll(() => server.close());

function renderDialog() {
  useAuth.setState({ user: { id: "u1", email: "a@b.co" }, accessToken: "tok" });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <CompareDialog
        open={true}
        onOpenChange={() => {}}
        conversationId="c1"
        models={models}
        prompt="compare this"
      />
    </QueryClientProvider>,
  );
}

describe("CompareDialog", () => {
  it("runs the comparison and activates the picked answer", async () => {
    const user = userEvent.setup();
    renderDialog();
    await user.click(await screen.findByRole("button", { name: "Compare 2 models" }));
    expect(await screen.findByText("first answer")).toBeInTheDocument();
    const picks = screen.getAllByRole("button", { name: "Pick this answer" });
    await user.click(picks[0]);
    await waitFor(() => expect(activated).toBe("a1"));
  });
});

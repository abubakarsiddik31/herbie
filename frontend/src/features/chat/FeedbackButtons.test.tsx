import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from "vitest";
import { FeedbackButtons } from "./FeedbackButtons";

let puts: { rating: string }[] = [];
let deletes = 0;

const server = setupServer(
  http.put("*/api/conversations/:id/messages/:messageId/feedback", async ({ request }) => {
    puts.push((await request.json()) as { rating: string });
    return new HttpResponse(null, { status: 204 });
  }),
  http.delete("*/api/conversations/:id/messages/:messageId/feedback", () => {
    deletes++;
    return new HttpResponse(null, { status: 204 });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => {
  puts = [];
  deletes = 0;
});
afterEach(() => {
  cleanup();
  server.resetHandlers();
});
afterAll(() => server.close());

function Harness({ initial }: { initial?: "up" | "down" }) {
  const [value, setValue] = useState<"up" | "down" | undefined>(initial);
  return <FeedbackButtons conversationId="c-1" messageId="m-1" value={value} onChange={setValue} />;
}

function renderButtons(initial?: "up" | "down") {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <Harness initial={initial} />
    </QueryClientProvider>,
  );
}

describe("FeedbackButtons", () => {
  it("rates, switches, and clears a message", async () => {
    renderButtons();
    const up = screen.getByRole("button", { name: "Good response" });
    const down = screen.getByRole("button", { name: "Bad response" });

    await userEvent.click(up);
    await waitFor(() => expect(puts).toEqual([{ rating: "up" }]));
    expect(up).toHaveAttribute("aria-pressed", "true");

    await userEvent.click(down);
    await waitFor(() => expect(puts).toEqual([{ rating: "up" }, { rating: "down" }]));
    expect(down).toHaveAttribute("aria-pressed", "true");
    expect(up).toHaveAttribute("aria-pressed", "false");

    await userEvent.click(down);
    await waitFor(() => expect(deletes).toBe(1));
    expect(down).toHaveAttribute("aria-pressed", "false");
  });

  it("restores the previous rating when the request fails", async () => {
    server.use(
      http.put("*/api/conversations/:id/messages/:messageId/feedback", () =>
        HttpResponse.json({ error: { code: "internal", message: "nope" } }, { status: 500 }),
      ),
    );
    renderButtons("up");
    const down = screen.getByRole("button", { name: "Bad response" });
    const up = screen.getByRole("button", { name: "Good response" });

    await userEvent.click(down);
    await waitFor(() => expect(up).toHaveAttribute("aria-pressed", "true"));
    expect(down).toHaveAttribute("aria-pressed", "false");
  });
});

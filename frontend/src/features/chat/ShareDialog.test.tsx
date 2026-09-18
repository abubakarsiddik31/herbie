import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { ShareDialog } from "./ShareDialog";

const server = setupServer(
  http.get("*/api/conversations/:id/shares", () => HttpResponse.json({ shared: false })),
  http.post("*/api/conversations/:id/shares", () =>
    HttpResponse.json({ token: "tok-123" }, { status: 201 }),
  ),
  http.delete("*/api/conversations/:id/shares", () => new HttpResponse(null, { status: 204 })),
);

beforeAll(() => {
  server.listen({ onUnhandledRequest: "error" });
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText: vi.fn().mockResolvedValue(undefined) },
  });
});
afterEach(() => {
  cleanup();
  server.resetHandlers();
});
afterAll(() => server.close());

function renderDialog() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ShareDialog conversation={{ id: "c1", title: "Thread" }} onOpenChange={() => {}} />
    </QueryClientProvider>,
  );
}

describe("ShareDialog", () => {
  it("creates a link and shows it with a working copy button", async () => {
    renderDialog();
    await userEvent.click(await screen.findByRole("button", { name: "Create link" }));
    const input = (await screen.findByLabelText("Share link")) as HTMLInputElement;
    expect(input.value).toContain("/s/tok-123");
    await userEvent.click(screen.getByRole("button", { name: "Copy" }));
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(input.value);
  });

  it("revokes an active link and returns to the unshared state", async () => {
    let shared = true;
    server.use(
      http.get("*/api/conversations/:id/shares", () => HttpResponse.json({ shared })),
      http.delete("*/api/conversations/:id/shares", () => {
        shared = false;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderDialog();
    expect(await screen.findByText(/already has an active link/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Revoke link" }));
    expect(await screen.findByRole("button", { name: "Create link" })).toBeInTheDocument();
  });
});

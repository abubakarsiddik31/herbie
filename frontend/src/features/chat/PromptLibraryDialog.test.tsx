import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import type { SavedPrompt } from "@/lib/types";
import { PromptLibraryDialog } from "./PromptLibraryDialog";

const stamp = "2026-01-01T00:00:00.000Z";

let prompts: SavedPrompt[] = [];
let posted: unknown[] = [];
let patched: string[] = [];
let deleted: string[] = [];

const server = setupServer(
  http.get("*/api/prompts", () => HttpResponse.json({ prompts })),
  http.post("*/api/prompts", async ({ request }) => {
    const body = (await request.json()) as { title: string; body: string };
    posted.push(body);
    const created = { id: `p-${prompts.length + 1}`, ...body, createdAt: stamp, updatedAt: stamp };
    prompts = [created, ...prompts];
    return HttpResponse.json(created, { status: 201 });
  }),
  http.patch("*/api/prompts/:id", async ({ request, params }) => {
    const id = params.id as string;
    patched.push(id);
    const body = (await request.json()) as { title?: string; body?: string };
    prompts = prompts.map((p) => (p.id === id ? { ...p, ...body } : p));
    return HttpResponse.json(prompts.find((p) => p.id === id));
  }),
  http.delete("*/api/prompts/:id", ({ params }) => {
    deleted.push(params.id as string);
    prompts = prompts.filter((p) => p.id !== params.id);
    return new HttpResponse(null, { status: 204 });
  }),
);

beforeAll(() => {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  server.listen({ onUnhandledRequest: "error" });
});
beforeEach(() => {
  prompts = [{ id: "p-1", title: "Standup", body: "Summarize yesterday", createdAt: stamp, updatedAt: stamp }];
  posted = [];
  patched = [];
  deleted = [];
});
afterEach(() => {
  cleanup();
  server.resetHandlers();
});
afterAll(() => server.close());

function renderDialog() {
  const onInsert = vi.fn();
  const onOpenChange = vi.fn();
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <PromptLibraryDialog open onOpenChange={onOpenChange} onInsert={onInsert} />
    </QueryClientProvider>,
  );
  return { onInsert, onOpenChange };
}

describe("PromptLibraryDialog", () => {
  it("lists prompts and inserts the selected body", async () => {
    const { onInsert, onOpenChange } = renderDialog();
    expect(await screen.findByText("Standup")).toBeInTheDocument();
    expect(screen.getByText("Summarize yesterday")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Insert" }));
    expect(onInsert).toHaveBeenCalledWith("Summarize yesterday");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("creates a prompt with title and body", async () => {
    renderDialog();
    await screen.findByText("Standup");
    await userEvent.click(screen.getByRole("button", { name: /New prompt/i }));
    await userEvent.type(screen.getByLabelText("Title"), "Daily recap");
    await userEvent.type(screen.getByLabelText("Prompt"), "Recap today's work");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toEqual({ title: "Daily recap", body: "Recap today's work" });
    expect(await screen.findByText("Daily recap")).toBeInTheDocument();
  });

  it("edits a prompt", async () => {
    renderDialog();
    await screen.findByText("Standup");
    await userEvent.click(screen.getByRole("button", { name: "Edit Standup" }));
    const title = screen.getByLabelText("Title");
    await userEvent.clear(title);
    await userEvent.type(title, "Renamed standup");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(patched).toEqual(["p-1"]));
    expect(await screen.findByText("Renamed standup")).toBeInTheDocument();
  });

  it("deletes a prompt after confirmation", async () => {
    renderDialog();
    await screen.findByText("Standup");
    await userEvent.click(screen.getByRole("button", { name: "Delete Standup" }));
    expect(screen.getByText(/Delete “Standup”\?/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(deleted).toEqual(["p-1"]));
    expect(await screen.findByText(/No saved prompts yet/i)).toBeInTheDocument();
  });
});

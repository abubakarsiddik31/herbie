import { act } from "react";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { toast } from "sonner";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { SidebarProvider } from "@/components/layout/SidebarContext";
import type { ChatMessage, Conversation } from "@/lib/types";
import { ChatPage } from "./ChatPage";

const createdAt = "2026-01-01T00:00:00.000Z";

interface Detail {
  conversation: Conversation;
  messages: ChatMessage[];
  context?: { estimatedTokens: number; thresholdTokens: number; keepRecent?: number; compacted?: boolean };
}

function conversation(overrides: Partial<Conversation> = {}): Conversation {
  return {
    id: "c1",
    title: "Test chat",
    createdAt,
    updatedAt: createdAt,
    model: "gpt-4o",
    temperature: 0.7,
    systemPrompt: "",
    ragEnabled: false,
    activeLeafId: null,
    ...overrides,
  };
}

function message(overrides: Partial<ChatMessage> & Pick<ChatMessage, "id" | "role" | "content">): ChatMessage {
  return { truncated: false, createdAt, ...overrides };
}

function detail(conversationOverrides: Partial<Conversation>, messages: ChatMessage[]): Detail {
  return { conversation: conversation(conversationOverrides), messages };
}

const sseBody = (frames: string[]) =>
  new ReadableStream({
    start(controller) {
      for (const frame of frames) controller.enqueue(new TextEncoder().encode(frame));
      controller.close();
    },
  });

function gatedStream() {
  let controller!: ReadableStreamDefaultController<Uint8Array>;
  const stream = new ReadableStream<Uint8Array>({
    start(c) {
      controller = c;
    },
  });
  return {
    stream,
    enqueue: (frame: string) => controller.enqueue(new TextEncoder().encode(frame)),
    close: () => controller.close(),
  };
}

const server = setupServer(
  http.get("*/api/conversations", () => HttpResponse.json([])),
  http.get("*/api/tools", () => HttpResponse.json([])),
  http.get("*/api/models", () =>
    HttpResponse.json({ default: "gpt-4o", models: [{ id: "gpt-4o", label: "GPT-4o", provider: "openai" }] }),
  ),
  http.get("*/api/documents", () => HttpResponse.json({ documents: [] })),
  http.get("*/api/tool-oauth/providers", () => HttpResponse.json({ providers: [] })),
  http.get("*/api/workflows", () => HttpResponse.json({ workflows: [] })),
  http.get("*/api/mcp/servers", () => HttpResponse.json({ servers: [] })),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  server.resetHandlers();
});
afterAll(() => server.close());

function LocationProbe() {
  const location = useLocation();
  return <div data-testid="location">{location.pathname}</div>;
}

function renderChat(initialPath = "/chat/c1") {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialPath]}>
        <SidebarProvider>
          <Routes>
            <Route path="/chat" element={<><LocationProbe /><ChatPage /></>} />
            <Route path="/chat/:conversationId" element={<><LocationProbe /><ChatPage /></>} />
          </Routes>
        </SidebarProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return queryClient;
}

describe("ChatPage branch switcher", () => {
  it("renders only when a message has more than one variant", async () => {
    server.use(
      http.get("*/api/conversations/c1", () =>
        HttpResponse.json(detail({ activeLeafId: "a1" }, [
          message({ id: "u1", role: "user", content: "hello" }),
          message({ id: "a1", role: "assistant", content: "only answer" }),
        ])),
      ),
    );
    renderChat();
    expect(await screen.findByText("only answer")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Next branch" })).not.toBeInTheDocument();
    expect(screen.queryByText("1/2")).not.toBeInTheDocument();
  });

  it("switches to the next variant and disables previous at index zero", async () => {
    const activated: string[] = [];
    server.use(
      http.get("*/api/conversations/c1", () =>
        HttpResponse.json(detail({ activeLeafId: "a-old" }, [
          message({ id: "u1", role: "user", content: "hello" }),
          message({
            id: "a-old",
            role: "assistant",
            content: "old answer",
            branch: { count: 2, index: 0, ids: ["a-old", "a-new"] },
          }),
        ])),
      ),
      http.post("*/api/conversations/c1/messages/:messageId/activate", ({ params }) => {
        activated.push(params.messageId as string);
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderChat();
    expect(await screen.findByText("old answer")).toBeInTheDocument();
    expect(screen.getByText("1/2")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Previous branch" })).toBeDisabled();

    await userEvent.click(screen.getByRole("button", { name: "Next branch" }));
    await waitFor(() => expect(activated).toEqual(["a-new"]));
  });

  it("switches to the previous variant and disables next at the end", async () => {
    const activated: string[] = [];
    server.use(
      http.get("*/api/conversations/c1", () =>
        HttpResponse.json(detail({ activeLeafId: "a-new" }, [
          message({ id: "u1", role: "user", content: "hello" }),
          message({
            id: "a-new",
            role: "assistant",
            content: "new answer",
            branch: { count: 2, index: 1, ids: ["a-old", "a-new"] },
          }),
        ])),
      ),
      http.post("*/api/conversations/c1/messages/:messageId/activate", ({ params }) => {
        activated.push(params.messageId as string);
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderChat();
    expect(await screen.findByText("new answer")).toBeInTheDocument();

    const nextButtons = screen.getAllByRole("button", { name: "Next branch" });
    for (const button of nextButtons) expect(button).toBeDisabled();

    await userEvent.click(screen.getByRole("button", { name: "Previous branch" }));
    await waitFor(() => expect(activated).toEqual(["a-old"]));
  });

  it("shows a toast when activation is blocked by an active run", async () => {
    const errorSpy = vi.spyOn(toast, "error").mockReturnValue("");
    server.use(
      http.get("*/api/conversations/c1", () =>
        HttpResponse.json(detail({ activeLeafId: "a-old" }, [
          message({
            id: "a-old",
            role: "assistant",
            content: "old answer",
            branch: { count: 2, index: 0, ids: ["a-old", "a-new"] },
          }),
        ])),
      ),
      http.post("*/api/conversations/c1/messages/:messageId/activate", () =>
        HttpResponse.json({ error: { code: "run_active", message: "run active" } }, { status: 409 }),
      ),
    );
    renderChat();
    await screen.findByText("old answer");
    await userEvent.click(screen.getByRole("button", { name: "Next branch" }));
    await waitFor(() =>
      expect(errorSpy).toHaveBeenCalledWith("A run is in progress — wait for it to finish"),
    );
  });

  it("hides branching and editing controls for bot conversations", async () => {
    server.use(
      http.get("*/api/conversations/c1", () =>
        HttpResponse.json(detail({ botId: "bot-1", activeLeafId: "a1" }, [
          message({ id: "u1", role: "user", content: "hello" }),
          message({
            id: "a1",
            role: "assistant",
            content: "bot answer",
            branch: { count: 2, index: 0, ids: ["a0", "a1"] },
          }),
        ])),
      ),
    );
    renderChat();
    expect(await screen.findByText("bot answer")).toBeInTheDocument();
    expect(screen.queryByText("1/2")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Previous branch" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit message" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Regenerate answer" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Fork conversation" })).not.toBeInTheDocument();
  });
});

describe("ChatPage reseeding", () => {
  it("reselects the thread when a refetch returns a new active leaf", async () => {
    let current = detail({ activeLeafId: "a-old" }, [
      message({ id: "u1", role: "user", content: "hello" }),
      message({ id: "a-old", role: "assistant", content: "old answer" }),
    ]);
    server.use(http.get("*/api/conversations/c1", () => HttpResponse.json(current)));
    const queryClient = renderChat();
    expect(await screen.findByText("old answer")).toBeInTheDocument();

    current = detail({ activeLeafId: "a-new" }, [
      message({ id: "u1", role: "user", content: "hello" }),
      message({ id: "a-new", role: "assistant", content: "new answer" }),
    ]);
    await act(async () => {
      await queryClient.refetchQueries({ queryKey: ["conversation", "c1"] });
    });

    expect(await screen.findByText("new answer")).toBeInTheDocument();
    expect(screen.queryByText("old answer")).not.toBeInTheDocument();
  });

  it("does not reseed while a run is streaming, then reseeds on completion", async () => {
    let current = detail({ activeLeafId: "a-old" }, [
      message({ id: "u-old", role: "user", content: "existing question" }),
      message({ id: "a-old", role: "assistant", content: "existing answer" }),
    ]);
    const stream = gatedStream();
    server.use(
      http.get("*/api/conversations/c1", () => HttpResponse.json(current)),
      http.post("*/api/conversations/c1/messages", () =>
        new HttpResponse(stream.stream, { headers: { "Content-Type": "text/event-stream" } }),
      ),
    );
    const queryClient = renderChat();
    expect(await screen.findByText("existing answer")).toBeInTheDocument();

    await userEvent.type(screen.getByPlaceholderText(/Message Herbie/), "local prompt");
    await userEvent.click(screen.getByRole("button", { name: "Send message" }));
    expect(await screen.findByRole("button", { name: "Stop generating" })).toBeInTheDocument();

    current = detail({ activeLeafId: "a-server" }, [
      message({ id: "u-server", role: "user", content: "server prompt" }),
      message({ id: "a-server", role: "assistant", content: "server replacement" }),
    ]);
    await act(async () => {
      await queryClient.refetchQueries({ queryKey: ["conversation", "c1"] });
    });

    expect(screen.getByText("local prompt")).toBeInTheDocument();
    expect(screen.queryByText("server replacement")).not.toBeInTheDocument();

    await act(async () => {
      stream.enqueue('event: delta\ndata: {"text":"streamed answer"}\n\n');
      stream.enqueue('event: done\ndata: {"messageId":"a-stream","inputTokens":1,"outputTokens":2,"requests":1,"costUsd":0}\n\n');
      stream.close();
    });
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Stop generating" })).not.toBeInTheDocument(),
    );
    expect(await screen.findByText("server replacement")).toBeInTheDocument();
  });

  it("targets the new user row on a second edit after the refetch remaps IDs", async () => {
    let edited = false;
    const editPaths: string[] = [];
    const initial = detail({ activeLeafId: "a-old" }, [
      message({ id: "u-old", role: "user", content: "original question" }),
      message({ id: "a-old", role: "assistant", content: "old answer" }),
    ]);
    const updated = detail({ activeLeafId: "a-new" }, [
      message({ id: "u-new", role: "user", content: "edited question" }),
      message({ id: "a-new", role: "assistant", content: "new answer" }),
    ]);
    server.use(
      http.get("*/api/conversations/c1", () => HttpResponse.json(edited ? updated : initial)),
      http.post("*/api/conversations/c1/messages/:messageId/edit", ({ params }) => {
        editPaths.push(params.messageId as string);
        edited = true;
        return new HttpResponse(
          sseBody([
            'event: delta\ndata: {"text":"streamed"}\n\n',
            'event: done\ndata: {"messageId":"a-new","inputTokens":1,"outputTokens":2,"requests":1,"costUsd":0}\n\n',
          ]),
          { headers: { "Content-Type": "text/event-stream" } },
        );
      }),
    );
    renderChat();
    expect(await screen.findByText("old answer")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Edit message" }));
    const editor = screen.getByDisplayValue("original question");
    await userEvent.clear(editor);
    await userEvent.type(editor, "edited question");
    await userEvent.click(screen.getByRole("button", { name: "Save & resend" }));
    await waitFor(() => expect(editPaths).toEqual(["u-old"]));
    expect(await screen.findByText("new answer")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Edit message" }));
    const secondEditor = screen.getByDisplayValue("edited question");
    await userEvent.clear(secondEditor);
    await userEvent.type(secondEditor, "second edit");
    await userEvent.click(screen.getByRole("button", { name: "Save & resend" }));
    await waitFor(() => expect(editPaths).toEqual(["u-old", "u-new"]));
  });
});

describe("ChatPage conversation actions", () => {
  it("forks the conversation and navigates to the new conversation", async () => {
    let forked = false;
    server.use(
      http.get("*/api/conversations/c1", () =>
        HttpResponse.json(detail({ activeLeafId: "a1" }, [
          message({ id: "u1", role: "user", content: "hello" }),
          message({ id: "a1", role: "assistant", content: "answer" }),
        ])),
      ),
      http.get("*/api/conversations/c2", () =>
        HttpResponse.json(detail({ id: "c2", title: "Fork of Test chat" }, [])),
      ),
      http.post("*/api/conversations/c1/fork", () => {
        forked = true;
        return HttpResponse.json(conversation({ id: "c2", title: "Fork of Test chat" }), { status: 201 });
      }),
    );
    renderChat();
    expect(await screen.findByText("answer")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Fork conversation" }));
    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent("/chat/c2"));
    expect(forked).toBe(true);
  });

  it("uses branch wording in the delete dialog and deletes the message", async () => {
    const deleted: string[] = [];
    server.use(
      http.get("*/api/conversations/c1", () =>
        HttpResponse.json(detail({ activeLeafId: "a1" }, [
          message({ id: "u1", role: "user", content: "hello" }),
          message({ id: "a1", role: "assistant", content: "answer" }),
        ])),
      ),
      http.delete("*/api/conversations/c1/messages/:messageId", ({ params }) => {
        deleted.push(params.messageId as string);
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderChat();
    expect(await screen.findByText("answer")).toBeInTheDocument();

    await userEvent.click(screen.getAllByRole("button", { name: "Delete message" })[0]);
    expect(screen.getByText("Delete this branch?")).toBeInTheDocument();
    expect(screen.getByText(/continuation in this branch/)).toBeInTheDocument();
    const dialog = screen.getByRole("dialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(deleted).toEqual(["u1"]));
  });

  it("shows a toast when deleting a branch is blocked", async () => {
    const errorSpy = vi.spyOn(toast, "error").mockReturnValue("");
    server.use(
      http.get("*/api/conversations/c1", () =>
        HttpResponse.json(detail({ activeLeafId: "a1" }, [
          message({ id: "u1", role: "user", content: "hello" }),
          message({ id: "a1", role: "assistant", content: "answer" }),
        ])),
      ),
      http.delete("*/api/conversations/c1/messages/:messageId", () =>
        HttpResponse.json({ error: { code: "approval_pending", message: "approval pending" } }, { status: 409 }),
      ),
    );
    renderChat();
    expect(await screen.findByText("answer")).toBeInTheDocument();

    await userEvent.click(screen.getAllByRole("button", { name: "Delete message" })[0]);
    const dialog = screen.getByRole("dialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(errorSpy).toHaveBeenCalledWith("Resolve the pending approval first"));
  });

  it("waits out an active run without leaving a streaming placeholder", async () => {
    server.use(
      http.get("*/api/conversations/c1/run", () => HttpResponse.json({ active: false })),
      http.get("*/api/conversations/c1", () =>
        HttpResponse.json(detail({ activeLeafId: "a1" }, [
          message({ id: "u1", role: "user", content: "hello" }),
          message({ id: "a1", role: "assistant", content: "answer" }),
        ])),
      ),
      http.post("*/api/conversations/c1/messages", () =>
        HttpResponse.json(
          { error: { code: "run_active", message: "A run is in progress" } },
          { status: 409 },
        ),
      ),
    );
    renderChat();
    expect(await screen.findByText("answer")).toBeInTheDocument();

    await userEvent.type(screen.getByPlaceholderText(/Message Herbie/), "another question");
    await userEvent.click(screen.getByRole("button", { name: "Send message" }));
    await waitFor(() =>
      expect(screen.queryByText("A run is in progress")).not.toBeInTheDocument(),
    );
    expect(screen.queryByRole("button", { name: "Stop generating" })).not.toBeInTheDocument();
  });

  it("shows an archived badge and clears it after sending", async () => {
    let archived = true;
    let sends = 0;
    server.use(
      http.get("*/api/conversations/c1", () =>
        HttpResponse.json(detail({ activeLeafId: "a1", archivedAt: archived ? createdAt : null }, [
          message({ id: "u1", role: "user", content: "hello" }),
          message({ id: "a1", role: "assistant", content: "answer" }),
        ])),
      ),
      http.post("*/api/conversations/c1/messages", () => {
        sends++;
        archived = false;
        return new HttpResponse(
          sseBody([
            'event: delta\ndata: {"text":"reply"}\n\n',
            'event: done\ndata: {"messageId":"a2","inputTokens":1,"outputTokens":1,"requests":1,"costUsd":0}\n\n',
          ]),
          { headers: { "Content-Type": "text/event-stream" } },
        );
      }),
    );
    renderChat();
    expect(await screen.findByText("Archived")).toBeInTheDocument();

    await userEvent.type(screen.getByPlaceholderText(/Message Herbie/), "continue");
    await userEvent.click(screen.getByRole("button", { name: "Send message" }));

    await waitFor(() => expect(sends).toBe(1));
    await waitFor(() => expect(screen.queryByText("Archived")).not.toBeInTheDocument());
  });
});

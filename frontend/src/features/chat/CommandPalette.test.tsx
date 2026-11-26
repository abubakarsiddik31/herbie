import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { waitFor } from "@testing-library/react";
import { CommandPalette } from "./CommandPalette";

const server = setupServer(
  http.get("*/api/search/messages", () => HttpResponse.json({ hits: [], nextOffset: null })),
);

function LocationProbe() {
  const location = useLocation();
  return <span data-testid="location">{location.pathname + location.search}</span>;
}

beforeAll(() => {
  server.listen({ onUnhandledRequest: "error" });
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
});

afterEach(() => {
  cleanup();
  server.resetHandlers();
});
afterAll(() => server.close());

function renderPalette(props: Partial<Parameters<typeof CommandPalette>[0]> = {}) {
  const onNewChat = vi.fn();
  const onExport = vi.fn();
  const onShare = vi.fn();
  const onOpenSettings = vi.fn();
  const onOpenShortcuts = vi.fn();
  const onOpenPromptLibrary = vi.fn();
  const onOpenChange = vi.fn();

  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <CommandPalette
        open={true}
        onOpenChange={onOpenChange}
        conversations={[
          {
            id: "c1",
            title: "Moon landing exploration",
            model: "gemini-2.5-flash",
            temperature: null,
            systemPrompt: "",
            ragEnabled: true,
            createdAt: "",
            updatedAt: "",
          },
        ]}
        activeConversation={{
          id: "c1",
          title: "Moon landing exploration",
          model: "gemini-2.5-flash",
          temperature: null,
          systemPrompt: "",
          ragEnabled: true,
          createdAt: "",
          updatedAt: "",
        }}
        onNewChat={onNewChat}
        onExport={onExport}
        onShare={onShare}
        onOpenSettings={onOpenSettings}
        onOpenShortcuts={onOpenShortcuts}
        onOpenPromptLibrary={onOpenPromptLibrary}
        {...props}
        />
        <LocationProbe />
      </MemoryRouter>
    </QueryClientProvider>,
  );

  return { onNewChat, onExport, onShare, onOpenSettings, onOpenShortcuts, onOpenPromptLibrary, onOpenChange };
}

describe("CommandPalette", () => {
  it("renders actions and recent conversations", () => {
    renderPalette();
    expect(screen.getByPlaceholderText(/type a command/i)).toBeInTheDocument();
    expect(screen.getByText("New chat")).toBeInTheDocument();
    expect(screen.getByText("Prompt library")).toBeInTheDocument();
    expect(screen.getByText("Moon landing exploration")).toBeInTheDocument();
  });

  it("filters items by search query", async () => {
    renderPalette();
    const input = screen.getByPlaceholderText(/type a command/i);
    await userEvent.type(input, "landing");
    expect(screen.getByText("Moon landing exploration")).toBeInTheDocument();
    expect(screen.queryByText("New chat")).not.toBeInTheDocument();
  });

  it("triggers action on click", async () => {
    const { onNewChat, onOpenChange } = renderPalette();
    await userEvent.click(screen.getByText("New chat"));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(onNewChat).toHaveBeenCalled();
  });

  it("activates the branch and jumps to a message hit", async () => {
    const activated: string[] = [];
    server.use(
      http.get("*/api/search/messages", () =>
        HttpResponse.json({
          hits: [
            {
              messageId: "m9",
              conversationId: "c1",
              title: "Deployment notes",
              role: "assistant",
              snippet: "lag \uE000replica\uE001",
              createdAt: "2026-01-01T00:00:00.000Z",
            },
          ],
          nextOffset: null,
        }),
      ),
      http.post("*/api/conversations/:id/messages/:messageId/activate", ({ params }) => {
        activated.push(`${params.id}/${params.messageId}`);
        return new HttpResponse(null, { status: 204 });
      }),
    );

    renderPalette();
    await userEvent.type(screen.getByPlaceholderText(/type a command/i), "replica");
    await userEvent.click(await screen.findByText("Deployment notes"));

    await waitFor(() => expect(activated).toEqual(["c1/m9"]));
    expect(screen.getByTestId("location").textContent).toBe("/chat/c1?m=m9");
    expect(screen.getByText("replica").tagName).toBe("MARK");
  });

  it("navigates with keyboard and runs with Enter", async () => {
    const { onOpenChange } = renderPalette();
    const input = screen.getByPlaceholderText(/type a command/i);
    await userEvent.type(input, "documents");
    await userEvent.keyboard("{Enter}");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("requests semantic reranking when the toggle is on", async () => {
    let semanticParam: string | null = null;
    server.use(
      http.get("*/api/search/messages", ({ request }) => {
        semanticParam = new URL(request.url).searchParams.get("semantic");
        return HttpResponse.json({ hits: [], nextOffset: null, semantic: true });
      }),
    );
    renderPalette({ semanticAvailable: true });
    await userEvent.type(screen.getByPlaceholderText(/type a command/i), "moon");
    await userEvent.click(screen.getByLabelText("Semantic message search"));
    await waitFor(() => expect(semanticParam).toBe("1"));
  });
});

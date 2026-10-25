import { cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { BotsPage } from "./BotsPage";
import { SidebarProvider } from "@/components/layout/SidebarContext";

const state = vi.hoisted(() => ({
  bots: [
    {
      id: "b1",
      name: "Scout",
      emoji: "🛰️",
      color: "sky",
      instructions: "track prices",
      model: "gemini-3.5-flash",
      temperature: null,
      ragEnabled: true,
      isArchived: false,
      conversationId: null,
      createdAt: "2026-10-25T00:00:00Z",
      updatedAt: "2026-10-25T00:00:00Z",
    },
  ],
}));

vi.mock("../features/bots/useBots", () => ({
  useBots: () => ({ data: { bots: state.bots, count: state.bots.length }, isLoading: false }),
  useBot: () => ({ data: undefined, isLoading: false }),
  useCreateBot: () => ({ isPending: false, mutate: vi.fn(), mutateAsync: vi.fn() }),
  useUpdateBot: () => ({ isPending: false, mutate: vi.fn(), mutateAsync: vi.fn() }),
  useDeleteBot: () => ({ isPending: false, mutate: vi.fn(), mutateAsync: vi.fn() }),
  useStartBotChat: () => ({ isPending: false, mutate: vi.fn(), mutateAsync: vi.fn() }),
}));

// BotFormDialog reads the model catalog on mount; keep the test offline.
vi.mock("../features/chat/useModels", () => ({
  useModels: () => ({
    data: {
      default: "gemini-3.5-flash",
      models: [{ id: "gemini-3.5-flash", label: "Gemini 3.5 Flash", provider: "gemini" }],
    },
  }),
}));

afterEach(() => {
  cleanup();
  state.bots = [
    {
      id: "b1",
      name: "Scout",
      emoji: "🛰️",
      color: "sky",
      instructions: "track prices",
      model: "gemini-3.5-flash",
      temperature: null,
      ragEnabled: true,
      isArchived: false,
      conversationId: null,
      createdAt: "2026-10-25T00:00:00Z",
      updatedAt: "2026-10-25T00:00:00Z",
    },
  ];
});

function renderBotsPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <SidebarProvider>
          <BotsPage />
        </SidebarProvider>
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe("BotsPage", () => {
  it("renders bots with name, job, and the New bot button", async () => {
    renderBotsPage();

    expect(await screen.findByText("Scout")).toBeInTheDocument();
    expect(screen.getByText("track prices")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /New bot/i })).toBeInTheDocument();
  });

  it("shows the friendly empty state when there are no bots", async () => {
    state.bots = [];
    renderBotsPage();

    expect(await screen.findByText("No bots yet")).toBeInTheDocument();
    expect(
      screen.getByText(
        "A bot is a teammate with a name, a job, and a model — it keeps its memory and keeps working after you close the tab."
      )
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Create your first bot/i })).toBeInTheDocument();
  });
});

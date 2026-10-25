import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { BotPage } from "./BotPage";
import { SidebarProvider } from "@/components/layout/SidebarContext";

const state = vi.hoisted(() => ({
  files: [
    { path: "notes/report.md", sizeBytes: 2048, updatedAt: "2026-10-24T12:00:00Z" },
  ],
  memories: [
    { id: "m1", content: "Prefers metric units", createdAt: "2026-10-24T10:00:00Z" },
  ],
  deleteMock: vi.fn(),
}));

vi.mock("../features/bots/useBots", () => ({
  isRunActive: (s: string) => ["queued", "running", "waiting_approval"].includes(s),
  useBot: () => ({
    data: {
      bot: {
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
      memories: state.memories,
    },
    isLoading: false,
  }),
  useBotRuns: () => ({ data: { runs: [] }, isLoading: false }),
  useStartBotChat: () => ({ isPending: false, mutate: vi.fn(), mutateAsync: vi.fn() }),
  useCreateBot: () => ({ isPending: false, mutate: vi.fn(), mutateAsync: vi.fn() }),
  useUpdateBot: () => ({ isPending: false, mutate: vi.fn(), mutateAsync: vi.fn() }),
  useBotRoutines: () => ({ data: { routines: [] }, isLoading: false }),
  useUpdateRoutine: () => ({ isPending: false, mutate: vi.fn(), mutateAsync: vi.fn() }),
  useDeleteRoutine: () => ({ isPending: false, mutate: vi.fn(), mutateAsync: vi.fn() }),
  useRunRoutineNow: () => ({ isPending: false, mutate: vi.fn(), mutateAsync: vi.fn() }),
  useBotFiles: () => ({ data: { files: state.files }, isLoading: false }),
  useBotMemories: () => ({ data: { memories: state.memories }, isLoading: false }),
  useDeleteBotMemory: () => ({ isPending: false, mutate: state.deleteMock, mutateAsync: vi.fn() }),
  useEnqueueTask: () => ({ isPending: false, mutate: vi.fn(), mutateAsync: vi.fn() }),
  useCancelRun: () => ({ isPending: false, mutate: vi.fn(), mutateAsync: vi.fn() }),
  useResolveRunApprovals: () => ({ isPending: false, mutate: vi.fn(), mutateAsync: vi.fn() }),
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
  state.deleteMock.mockClear();
});

function renderBotPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/bots/b1"]}>
        <SidebarProvider>
          <BotPage />
        </SidebarProvider>
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe("BotPage workspace", () => {
  it("renders the file and memory rows on the Workspace tab", async () => {
    renderBotPage();

    expect(await screen.findByText("Scout")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Workspace" }));

    expect(screen.getByText("notes/report.md")).toBeInTheDocument();
    expect(screen.getByText(/2\.0 KB · /)).toBeInTheDocument();
    expect(
      screen.getByText("Bots write these via the write_file tool.")
    ).toBeInTheDocument();
    expect(screen.getByText("Prefers metric units")).toBeInTheDocument();
    expect(
      screen.getByText("Learned via the remember tool — injected into every run.")
    ).toBeInTheDocument();
  });

  it("deletes a memory via the mutation when the trash button is clicked", async () => {
    renderBotPage();

    expect(await screen.findByText("Scout")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Workspace" }));

    fireEvent.click(screen.getByRole("button", { name: "Delete memory" }));
    expect(state.deleteMock).toHaveBeenCalledWith("m1", expect.anything());
  });
});

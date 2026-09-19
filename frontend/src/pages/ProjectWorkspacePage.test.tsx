import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { ProjectWorkspacePage } from "./ProjectWorkspacePage";
import { SidebarProvider } from "@/components/layout/SidebarContext";

const mockProject = {
  project: {
    id: "proj-1",
    name: "Autonomous Agent Project",
    description: "Multi-agent research and development",
    customInstructions: "",
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
  },
  files: [],
  conversations: [
    {
      id: "conv-1",
      title: "Initial Planning Discussion",
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
      model: "gpt-4o",
      temperature: 0.7,
      systemPrompt: "",
      ragEnabled: false,
    },
  ],
};

const mockDetail = {
  id: "conv-1",
  title: "Initial Planning Discussion",
  createdAt: new Date().toISOString(),
  updatedAt: new Date().toISOString(),
  model: "gpt-4o",
  temperature: 0.7,
  systemPrompt: "",
  ragEnabled: false,
  messages: [
    {
      id: "m-1",
      role: "assistant",
      content: "Hello! Ready to work on Autonomous Agent Project.",
      createdAt: new Date().toISOString(),
    },
  ],
};

const server = setupServer(
  http.get("*/api/projects/proj-1", () => HttpResponse.json(mockProject)),
  http.get("*/api/conversations/conv-1", () => HttpResponse.json(mockDetail)),
  http.get("*/api/tools", () => HttpResponse.json([])),
  http.get("*/api/profile", () => HttpResponse.json({ email: "user@example.com" })),
);

beforeAll(() => server.listen());
afterEach(() => {
  cleanup();
  server.resetHandlers();
});
afterAll(() => server.close());

function renderProjectWorkspace(initialPath = "/projects/proj-1") {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialPath]}>
        <SidebarProvider>
          <Routes>
            <Route path="/projects/:projectId" element={<ProjectWorkspacePage />} />
          </Routes>
        </SidebarProvider>
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe("ProjectWorkspacePage - New Chat UX", () => {
  it("renders workspace header with conversation switcher and New chat action", async () => {
    const user = userEvent.setup();
    renderProjectWorkspace();

    // Project name is in header
    expect(await screen.findByText("Autonomous Agent Project")).toBeInTheDocument();

    // Prominent "New chat" button in header
    const newChatBtn = screen.getByRole("button", { name: /New chat/i });
    expect(newChatBtn).toBeInTheDocument();

    // Conversation switcher displays current active conversation title
    expect(await screen.findByText("Initial Planning Discussion")).toBeInTheDocument();

    // Clicking New chat switches to clean workspace state for fresh conversation
    await user.click(newChatBtn);

    // Empty state should be visible for the new chat
    expect(await screen.findByText("Autonomous Agent Project Workspace")).toBeInTheDocument();
  });
});

import { cleanup, render, screen } from "@testing-library/react";
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
      usage: {
        inputTokens: 120,
        outputTokens: 450,
        costUsd: 0.00075,
        model: "gemini-3.5-flash",
      },
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

describe("ProjectWorkspacePage - Clean Header & New Chat Deep Link", () => {
  it("renders workspace header with conversation switcher and no redundant header new chat button", async () => {
    renderProjectWorkspace();

    // Project name is in header
    expect(await screen.findByText("Autonomous Agent Project")).toBeInTheDocument();

    // Conversation switcher displays current active conversation title
    expect(await screen.findByText("Initial Planning Discussion")).toBeInTheDocument();

    // No redundant prominent "New chat" button in workspace header
    expect(screen.queryByRole("button", { name: /^New chat$/i })).not.toBeInTheDocument();
  });

  it("renders clean workspace state for fresh conversation when loaded with ?c=new", async () => {
    renderProjectWorkspace("/projects/proj-1?c=new");

    // Project name is in header
    expect(await screen.findByText("Autonomous Agent Project")).toBeInTheDocument();

    // Empty state should be visible for the new chat
    expect(await screen.findByText("Autonomous Agent Project Workspace")).toBeInTheDocument();
  });

  it("renders assistant message with usage tokens, cost, and model metadata", async () => {
    renderProjectWorkspace();

    expect(await screen.findByText("Hello! Ready to work on Autonomous Agent Project.")).toBeInTheDocument();

    // Verify token usage, cost, and model are displayed
    expect(screen.getByText(/120 in \/ 450 out · \$0\.00075 · gemini-3\.5-flash/)).toBeInTheDocument();

    // Verify Copy and Delete action buttons are present
    expect(screen.getByRole("button", { name: "Copy message" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete message" })).toBeInTheDocument();
  });
});

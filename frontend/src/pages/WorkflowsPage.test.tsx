import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter } from "react-router";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { WorkflowsPage } from "./WorkflowsPage";
import type { Workflow } from "@/lib/types";

let mockWorkflows: Workflow[] = [];

const server = setupServer(
  http.get("*/api/workflows", () => {
    return HttpResponse.json({ workflows: mockWorkflows });
  }),
  http.post("*/api/workflows", async ({ request }) => {
    const body = (await request.json()) as Partial<Workflow>;
    const created: Workflow = {
      id: `wf-${mockWorkflows.length + 1}`,
      name: body.name || "Untitled",
      description: body.description || "",
      triggerType: body.triggerType || "manual",
      webhookSlug: body.webhookSlug || null,
      nodes: body.nodes || [],
      edges: body.edges || [],
      exposeAsTool: body.exposeAsTool ?? false,
      toolName: body.toolName || "",
      toolDescription: body.toolDescription || "",
      isActive: body.isActive ?? true,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    };
    mockWorkflows.push(created);
    return HttpResponse.json(created, { status: 201 });
  }),
  http.patch("*/api/workflows/:id", async ({ params, request }) => {
    const body = (await request.json()) as Partial<Workflow>;
    const target = mockWorkflows.find((w) => w.id === params.id);
    if (target) {
      Object.assign(target, body);
      return HttpResponse.json(target);
    }
    return new HttpResponse(null, { status: 404 });
  }),
  http.delete("*/api/workflows/:id", ({ params }) => {
    mockWorkflows = mockWorkflows.filter((w) => w.id !== params.id);
    return new HttpResponse(null, { status: 204 });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  cleanup();
  server.resetHandlers();
  mockWorkflows = [];
});
afterAll(() => server.close());

function renderWorkflowsPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <WorkflowsPage />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe("WorkflowsPage", () => {
  it("renders empty state and allows creating a workflow", async () => {
    const user = userEvent.setup();
    renderWorkflowsPage();

    expect(await screen.findByText("No workflows created yet")).toBeInTheDocument();

    // Click "New Workflow"
    const newBtn = screen.getByRole("button", { name: /New Workflow/i });
    await user.click(newBtn);

    // Fill dialog
    const nameInput = screen.getByLabelText(/Workflow Name/i);
    await user.type(nameInput, "Slack Alerts Flow");

    const descInput = screen.getByLabelText(/Description/i);
    await user.type(descInput, "Sends notifications on alerts");

    const submitBtn = screen.getByRole("button", { name: /Create & Open Canvas/i });
    await user.click(submitBtn);

    await waitFor(() => {
      expect(mockWorkflows).toHaveLength(1);
      expect(mockWorkflows[0].name).toBe("Slack Alerts Flow");
    });
  });

  it("lists existing workflows and filters them via search", async () => {
    mockWorkflows = [
      {
        id: "wf-1",
        name: "Sync GitHub Issues",
        description: "Fetch new tickets and save",
        triggerType: "manual",
        nodes: [],
        edges: [],
        exposeAsTool: false,
        toolName: "",
        toolDescription: "",
        isActive: true,
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      },
      {
        id: "wf-2",
        name: "Daily Weather Digest",
        description: "Email temperature every morning",
        triggerType: "manual",
        nodes: [],
        edges: [],
        exposeAsTool: false,
        toolName: "",
        toolDescription: "",
        isActive: false,
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      },
    ];

    const user = userEvent.setup();
    renderWorkflowsPage();

    expect(await screen.findByText("Sync GitHub Issues")).toBeInTheDocument();
    expect(screen.getByText("Daily Weather Digest")).toBeInTheDocument();

    // Filter by "GitHub"
    const searchInput = screen.getByPlaceholderText(/Search my workflows/i);
    await user.type(searchInput, "GitHub");

    expect(screen.getByText("Sync GitHub Issues")).toBeInTheDocument();
    expect(screen.queryByText("Daily Weather Digest")).not.toBeInTheDocument();
  });

  it("switches to Example Templates tab, filters by category, previews and clones a template", async () => {
    const user = userEvent.setup();
    renderWorkflowsPage();

    // Switch to Example Templates tab
    const templatesTab = screen.getByRole("button", { name: /Example Templates/i });
    await user.click(templatesTab);

    // Verify templates are rendered
    expect(await screen.findByText("GitHub Issue Triage & AI Summary")).toBeInTheDocument();
    expect(screen.getByText("Hacker News AI Research Digest")).toBeInTheDocument();

    // Filter by category "Alerts & Messaging"
    const alertsCategoryBtn = screen.getByRole("button", { name: "Alerts & Messaging" });
    await user.click(alertsCategoryBtn);

    expect(screen.getByText("Crypto Price Threshold Monitor")).toBeInTheDocument();
    expect(screen.queryByText("Hacker News AI Research Digest")).not.toBeInTheDocument();

    // Search within templates
    const searchInput = screen.getByPlaceholderText(/Search templates/i);
    await user.type(searchInput, "Crypto");

    expect(screen.getByText("Crypto Price Threshold Monitor")).toBeInTheDocument();

    // Click "Preview"
    const previewBtn = screen.getByRole("button", { name: /Preview/i });
    await user.click(previewBtn);

    // Verify preview modal appears
    expect(await screen.findByText("Pipeline Execution Steps (5 nodes)")).toBeInTheDocument();

    // Click "Use This Template"
    const useBtn = screen.getByRole("button", { name: /Use This Template/i });
    await user.click(useBtn);

    await waitFor(() => {
      expect(mockWorkflows).toHaveLength(1);
      expect(mockWorkflows[0].name).toBe("Crypto Price Threshold Monitor");
    });
  });
});

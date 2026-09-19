import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { CredentialsDialog } from "./CredentialsDialog";

const mockCredentials = [
  {
    id: "cred-1",
    name: "github_pat",
    type: "bearer_token",
    data: { token: "••••••••" },
    createdAt: "2026-09-19T00:00:00Z",
    updatedAt: "2026-09-19T00:00:00Z",
  },
];

const mockProviders = [
  {
    id: "github",
    name: "GitHub",
    configured: true,
    connected: false,
    scopes: ["repo", "read:user"],
  },
  {
    id: "slack",
    name: "Slack",
    configured: true,
    connected: true,
    credentialName: "slack",
    scopes: ["incoming-webhook", "chat:write"],
    connectedAt: "2026-09-19T00:00:00Z",
  },
];

const mockAudits = [
  {
    id: "aud-1",
    userId: "u1",
    callerType: "chat_agent",
    callerId: "conv-1",
    toolName: "web_search",
    action: "execute",
    inputSummary: '{"q":"golang release"}',
    outputSummary: "Found Go 1.23",
    status: "success",
    durationMs: 250,
    createdAt: "2026-09-19T10:00:00Z",
  },
];

const server = setupServer(
  http.get("*/api/workflow-credentials", () => HttpResponse.json({ credentials: mockCredentials })),
  http.post("*/api/workflow-credentials", async ({ request }) => {
    const body = (await request.json()) as { name: string; type: string };
    return HttpResponse.json({
      id: "cred-2",
      name: body.name,
      type: body.type,
      data: { token: "••••••••" },
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    }, { status: 201 });
  }),
  http.delete("*/api/workflow-credentials/:id", () => new HttpResponse(null, { status: 204 })),
  http.get("*/api/tool-oauth/providers", () => HttpResponse.json({ providers: mockProviders })),
  http.post("*/api/tool-oauth/:provider/disconnect", () => HttpResponse.json({ ok: true })),
  http.get("*/api/tool-audit-logs", () => HttpResponse.json({ audits: mockAudits })),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  cleanup();
  server.resetHandlers();
});
afterAll(() => server.close());

function renderDialog(open = true) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <CredentialsDialog open={open} onOpenChange={() => {}} />
    </QueryClientProvider>,
  );
}

describe("CredentialsDialog", () => {
  it("renders credentials vault with AES-256 banner and OAuth cards", async () => {
    renderDialog();
    expect(await screen.findByText("Credentials Vault & Tool Safety")).toBeInTheDocument();
    expect(screen.getByText("AES-256-GCM Encrypted at Rest")).toBeInTheDocument();

    // Verify OAuth provider cards
    expect(screen.getByText("One-Click OAuth Integrations")).toBeInTheDocument();
    expect(screen.getByText("GitHub")).toBeInTheDocument();
    expect(screen.getByText("Slack")).toBeInTheDocument();
    expect(await screen.findByText("Connected")).toBeInTheDocument(); // Slack is connected

    // Verify stored credential
    expect(await screen.findByText("github_pat")).toBeInTheDocument();
  });

  it("switches to Security Audit Trail tab and displays logs", async () => {
    renderDialog();
    const auditTabBtn = await screen.findByRole("button", { name: /Security Audit Trail/i });
    await userEvent.click(auditTabBtn);

    expect(await screen.findByText("Recent Security Invocations")).toBeInTheDocument();
    expect(screen.getByText("web_search")).toBeInTheDocument();
    expect(screen.getByText("AI Agent")).toBeInTheDocument();
    expect(screen.getByText("success")).toBeInTheDocument();
  });
});

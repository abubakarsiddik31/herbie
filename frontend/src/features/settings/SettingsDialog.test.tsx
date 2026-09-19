import { cleanup, render, screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { SettingsDialog, type SettingsTab } from "./SettingsDialog";

const mockProviders = [
  { id: "google_calendar", name: "Google Calendar", connected: false, configured: true },
  { id: "github", name: "GitHub", connected: true, configured: true },
  { id: "slack", name: "Slack", connected: false, configured: true },
];

const server = setupServer(
  http.get("*/api/profile", () => HttpResponse.json({ email: "user@example.com", custom_instructions: "" })),
  http.get("*/api/memories", () => HttpResponse.json({ memories: [] })),
  http.get("*/api/tools", () => HttpResponse.json([])),
  http.get("*/api/documents", () => HttpResponse.json({ documents: [] })),
  http.get("*/api/tool-oauth/providers", () => HttpResponse.json({ providers: mockProviders })),
  http.get("*/api/workflows", () => HttpResponse.json({ workflows: [] })),
  http.get("*/api/mcp/servers", () => HttpResponse.json({ servers: [] })),
);

beforeAll(() => server.listen());
afterEach(() => {
  cleanup();
  server.resetHandlers();
});
afterAll(() => server.close());

function renderSettingsDialog(props: { open?: boolean; defaultTab?: SettingsTab } = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <SettingsDialog
          open={props.open ?? true}
          onOpenChange={() => {}}
          defaultTab={props.defaultTab ?? "tools"}
        />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe("SettingsDialog - Integrated Apps & Tools", () => {
  it("renders apps without any 'Not Connected' badge, showing green Connect for unconnected and Connected badge for connected apps", async () => {
    renderSettingsDialog({ defaultTab: "tools" });

    // Section title
    expect(await screen.findByText("Integrated Apps & Tools")).toBeInTheDocument();

    // App titles and handles are clearly visible
    expect(screen.getByText("Google Calendar")).toBeInTheDocument();
    expect(screen.getByText("@calendar")).toBeInTheDocument();
    expect(screen.getByText("GitHub")).toBeInTheDocument();
    expect(screen.getByText("@github")).toBeInTheDocument();
    expect(screen.getByText("Slack")).toBeInTheDocument();
    expect(screen.getByText("@slack")).toBeInTheDocument();
    expect(screen.getByText("Web Search")).toBeInTheDocument();
    expect(screen.getByText("@web")).toBeInTheDocument();

    // Built-in badge for Web Search
    expect(screen.getByText("Built-in")).toBeInTheDocument();

    // CRITICAL: "Not Connected" should NOT exist anywhere in the view
    expect(screen.queryByText("Not Connected")).not.toBeInTheDocument();

    // GitHub is connected -> shows "Connected" badge and "Disconnect" button
    expect(await screen.findByText("Connected")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Disconnect" })).toBeInTheDocument();

    // Code Sandbox and Web Reader are clearly visible with handles
    expect(screen.getByText("Code Sandbox")).toBeInTheDocument();
    expect(screen.getByText("@code_runner")).toBeInTheDocument();
    expect(screen.getByText("Web Reader")).toBeInTheDocument();
    expect(screen.getByText("@web_fetch")).toBeInTheDocument();

    // Google Calendar and Slack are unconnected -> show green "Connect" buttons
    const connectButtons = screen.getAllByRole("button", { name: "Connect" });
    expect(connectButtons.length).toBeGreaterThanOrEqual(4);
    // Verify connect button has emerald / green styling
    expect(connectButtons[0].className).toContain("bg-emerald-600");
  });
});

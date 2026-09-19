import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import type { ReactNode } from "react";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { useChatApps, extractAppConnectProviders } from "./useChatApps";

const server = setupServer(
  http.get("*/api/tool-oauth/providers", () =>
    HttpResponse.json({
      providers: [
        {
          id: "google_calendar",
          name: "Google Calendar",
          configured: true,
          connected: true,
          scopes: ["calendar.events", "calendar.readonly"],
        },
        {
          id: "github",
          name: "GitHub",
          configured: true,
          connected: false,
          scopes: ["repo"],
        },
        {
          id: "slack",
          name: "Slack",
          configured: false,
          connected: false,
        },
      ],
    }),
  ),
  http.get("*/api/workflows", () =>
    HttpResponse.json({
      workflows: [
        {
          id: "wf-1",
          name: "Deploy Production",
          toolName: "deploy_prod",
          toolDescription: "Trigger deployment",
          exposeAsTool: true,
          nodes: [],
          edges: [],
          createdAt: "2026-09-19T00:00:00Z",
          updatedAt: "2026-09-19T00:00:00Z",
        },
      ],
    }),
  ),
  http.get("*/api/tools", () => HttpResponse.json([])),
  http.get("*/api/mcp/servers", () => HttpResponse.json({ servers: [] })),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function wrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe("useChatApps", () => {
  it("populates apps with OAuth providers, built-ins, and active workflow tools", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useChatApps(), { wrapper: wrapper(client) });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    const apps = result.current.apps;
    const gcal = apps.find((a) => a.id === "google_calendar");
    expect(gcal).toBeDefined();
    expect(gcal?.mention).toBe("calendar");
    expect(gcal?.connected).toBe(true);

    const gh = apps.find((a) => a.id === "github");
    expect(gh).toBeDefined();
    expect(gh?.connected).toBe(false);
    expect(gh?.requiresConnection).toBe(true);

    const web = apps.find((a) => a.id === "web_search");
    expect(web).toBeDefined();
    expect(web?.connected).toBe(true);
    expect(web?.requiresConnection).toBe(false);

    const wf = apps.find((a) => a.id === "workflow_wf-1");
    expect(wf).toBeDefined();
    expect(wf?.name).toBe("Deploy Production");
    expect(wf?.type).toBe("workflow");
  });

  it("filters apps by search query", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useChatApps(), { wrapper: wrapper(client) });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    const calMatches = result.current.filterApps("cal");
    expect(calMatches.some((a) => a.id === "google_calendar")).toBe(true);
    expect(calMatches.some((a) => a.id === "github")).toBe(false);

    const webMatches = result.current.filterApps("web");
    expect(webMatches.some((a) => a.id === "web_search")).toBe(true);
  });

  it("looks up app by mention", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useChatApps(), { wrapper: wrapper(client) });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    const app = result.current.getAppByMention("@calendar");
    expect(app?.id).toBe("google_calendar");

    const appWithoutAt = result.current.getAppByMention("github");
    expect(appWithoutAt?.id).toBe("github");
  });

  it("extracts app connect providers from text", () => {
    const text =
      "Please connect your calendar: [Connect Google Calendar](connect:google_calendar) or connect:github to proceed.";
    const providers = extractAppConnectProviders(text);
    expect(providers).toEqual(["google_calendar", "github"]);
  });
});

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import type { ReactNode } from "react";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { AppConnectCard } from "./AppConnectCard";

let startCalledWith: string | null = null;
let linkCalledWith: string | null = null;

const server = setupServer(
  http.get("*/api/mcp/servers", () =>
    HttpResponse.json({
      servers: [],
    }),
  ),
  http.get("*/api/mcp/catalog", () =>
    HttpResponse.json({
      catalog: [],
    }),
  ),
  http.post("*/api/mcp/catalog/:appId/link", ({ params }) => {
    linkCalledWith = params.appId as string;
    return HttpResponse.json({ ok: true });
  }),
  http.get("*/api/tool-oauth/providers", () =>
    HttpResponse.json({
      providers: [
        {
          id: "google_calendar",
          name: "Google Calendar",
          configured: true,
          connected: false,
        },
      ],
    }),
  ),
  http.get("*/api/tool-oauth/:provider/start", ({ params, request }) => {
    const url = new URL(request.url);
    startCalledWith = `${params.provider}?return_to=${url.searchParams.get("return_to")}`;
    return HttpResponse.json({ url: "https://accounts.google.com/o/oauth2/v2/auth?test=1" });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  server.resetHandlers();
  startCalledWith = null;
  linkCalledWith = null;
});
afterAll(() => server.close());

function wrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe("AppConnectCard", () => {
  it("renders connect prompt for unlinked provider and triggers OAuth start", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<AppConnectCard providerId="google_calendar" returnTo="/chat/test-123" />, {
      wrapper: wrapper(client),
    });

    expect(await screen.findByText("Google Calendar Connection Required")).toBeInTheDocument();
    expect(screen.getByText("OAuth Connect")).toBeInTheDocument();
    expect(screen.getByText("1-Click Link")).toBeInTheDocument();

    // Stub window.location.href assignment
    const btn = screen.getByText("OAuth Connect");
    fireEvent.click(btn);

    await waitFor(() => {
      expect(startCalledWith).toContain("google_calendar");
      expect(startCalledWith).toContain("return_to=/chat/test-123");
    });
  });

  it("triggers 1-click link when clicking 1-Click Link button", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<AppConnectCard providerId="github" />, {
      wrapper: wrapper(client),
    });

    const linkBtn = await screen.findByText("1-Click Link");
    fireEvent.click(linkBtn);

    await waitFor(() => {
      expect(linkCalledWith).toBe("github");
    });
  });

  it("renders connected badge when provider is connected", async () => {
    server.use(
      http.get("*/api/tool-oauth/providers", () =>
        HttpResponse.json({
          providers: [
            {
              id: "google_calendar",
              name: "Google Calendar",
              configured: true,
              connected: true,
            },
          ],
        }),
      ),
    );

    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<AppConnectCard providerId="google_calendar" />, {
      wrapper: wrapper(client),
    });

    expect(await screen.findByText("Connected")).toBeInTheDocument();
    expect(screen.getByText("Ready for chat prompts and automated actions.")).toBeInTheDocument();
  });
});

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import type { ReactNode } from "react";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { OAuthPopupDialog } from "./OAuthPopupDialog";

let configCalledPayload: unknown = null;

const server = setupServer(
  http.get("*/api/tool-oauth/providers", () =>
    HttpResponse.json({
      providers: [
        {
          id: "google_calendar",
          name: "Google Calendar",
          configured: false,
          connected: false,
        },
      ],
    }),
  ),
  http.post("*/api/tool-oauth/:provider/config", async ({ request }) => {
    configCalledPayload = await request.json();
    return HttpResponse.json({ ok: true });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  server.resetHandlers();
  configCalledPayload = null;
});
afterAll(() => server.close());

function wrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe("OAuthPopupDialog", () => {
  it("renders OAuth setup form when unconfigured and displays callback URI", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<OAuthPopupDialog open={true} onOpenChange={vi.fn()} providerId="google_calendar" />, {
      wrapper: wrapper(client),
    });

    expect(await screen.findByText("Connect Google Calendar")).toBeInTheDocument();
    expect(screen.getByText("OAuth Credentials Required")).toBeInTheDocument();
    expect(screen.getByLabelText("Client ID")).toBeInTheDocument();
    expect(screen.getByLabelText("Client Secret")).toBeInTheDocument();
    expect(screen.getByText(/api\/tool-oauth\/google_calendar\/callback/)).toBeInTheDocument();
  });

  it("allows switching to Direct Token tab and saving access token", async () => {
    const onOpenChange = vi.fn();
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<OAuthPopupDialog open={true} onOpenChange={onOpenChange} providerId="google_calendar" />, {
      wrapper: wrapper(client),
    });

    const tokenTabBtn = screen.getByRole("button", { name: "Direct Token" });
    fireEvent.click(tokenTabBtn);

    expect(screen.getByLabelText("Access Token / API Key")).toBeInTheDocument();
    const input = screen.getByLabelText("Access Token / API Key");
    fireEvent.change(input, { target: { value: "ya29.mock-google-token" } });

    const saveBtn = screen.getByText("Save & Connect");
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(configCalledPayload).toMatchObject({
        token: "ya29.mock-google-token",
      });
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });
  });
});

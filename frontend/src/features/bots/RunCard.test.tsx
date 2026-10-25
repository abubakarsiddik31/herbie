import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { BotRun, PendingCall } from "@/lib/types";
import { RunCard } from "./RunCard";

const h = vi.hoisted(() => {
  const run: BotRun = {
    id: "run-1",
    botId: "b1",
    conversationId: "conv-1",
    status: "waiting_approval",
    task: "Summarize today's AI news and email me the digest",
    triggerSource: "manual",
    error: null,
    activity: [
      { event: "tool_start", name: "web_fetch", t: 1 },
      { event: "tool_end", name: "web_fetch", ok: true, t: 2 },
    ],
    inputTokens: 120,
    outputTokens: 340,
    requests: 2,
    costUsd: 0.0021,
    createdAt: "2026-10-25T10:00:00Z",
    startedAt: "2026-10-25T10:00:01Z",
    finishedAt: null,
  };
  const pending: PendingCall[] = [
    {
      callId: "call-1",
      toolName: "send_email",
      args: { to: "me@example.com", subject: "AI digest" },
      reason: "Sends an email on your behalf",
    },
  ];
  return {
    run,
    pending,
    resolve: vi.fn(),
    cancel: vi.fn(),
  };
});

vi.mock("./useBots", () => ({
  useBotRun: () => ({ data: { run: h.run, pending: h.pending }, isLoading: false }),
  useCancelRun: () => ({ isPending: false, mutate: h.cancel }),
  useResolveRunApprovals: () => ({ isPending: false, mutate: h.resolve }),
}));

afterEach(() => {
  cleanup();
  h.resolve.mockClear();
  h.cancel.mockClear();
});

function renderRunCard(run: BotRun = h.run) {
  return render(
    <MemoryRouter>
      <RunCard run={run} />
    </MemoryRouter>
  );
}

describe("RunCard", () => {
  it("shows the waiting status and the pending tool with Approve and Deny", () => {
    renderRunCard();

    expect(screen.getByText("Waiting for approval")).toBeInTheDocument();
    expect(screen.getByText("send_email")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Approve" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Deny" })).toBeInTheDocument();
  });

  it("approves all pending calls at once", () => {
    renderRunCard();

    fireEvent.click(screen.getByRole("button", { name: "Approve" }));

    expect(h.resolve).toHaveBeenCalledTimes(1);
    expect(h.resolve.mock.calls[0]?.[0]).toEqual(
      expect.objectContaining({ decisions: [{ callId: "call-1", approved: true }] })
    );
  });

  it("denies all pending calls at once", () => {
    renderRunCard();

    fireEvent.click(screen.getByRole("button", { name: "Deny" }));

    expect(h.resolve).toHaveBeenCalledTimes(1);
    expect(h.resolve.mock.calls[0]?.[0]).toEqual(
      expect.objectContaining({ decisions: [{ callId: "call-1", approved: false }] })
    );
  });

  it("offers to open the run's conversation", () => {
    renderRunCard();

    expect(screen.getByRole("link", { name: /open conversation/i })).toHaveAttribute(
      "href",
      "/chat/conv-1"
    );
  });

  it("shows the cancel button for a parked (waiting_approval) run", () => {
    renderRunCard();

    expect(screen.getByRole("button", { name: "Cancel run" })).toBeInTheDocument();
  });

  it("cancels a parked run via the cancel button", () => {
    renderRunCard();

    fireEvent.click(screen.getByRole("button", { name: "Cancel run" }));

    expect(h.cancel).toHaveBeenCalledWith("run-1", expect.anything());
  });
});

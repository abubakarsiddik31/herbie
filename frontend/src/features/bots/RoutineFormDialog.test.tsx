import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import { RoutineFormDialog } from "./RoutineFormDialog";

const h = vi.hoisted(() => ({
  create: vi.fn(),
  update: vi.fn(),
}));

vi.mock("./useBots", () => ({
  useCreateRoutine: () => ({ isPending: false, mutateAsync: h.create }),
  useUpdateRoutine: () => ({ isPending: false, mutateAsync: h.update }),
}));

afterEach(() => {
  cleanup();
  h.create.mockReset();
  h.update.mockReset();
});

describe("RoutineFormDialog", () => {
  it("submits a new routine with the form fields", async () => {
    h.create.mockResolvedValue(undefined);
    render(
      <RoutineFormDialog open onOpenChange={vi.fn()} botId="bot-1" />
    );

    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "Morning digest" },
    });
    fireEvent.change(screen.getByLabelText("Schedule"), {
      target: { value: "0 8 * * 1-5" },
    });
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Summarize yesterday's emails." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create routine" }));

    await waitFor(() => expect(h.create).toHaveBeenCalledTimes(1));
    expect(h.create.mock.calls[0]?.[0]).toEqual({
      name: "Morning digest",
      schedule: "0 8 * * 1-5",
      timezone: "UTC",
      prompt: "Summarize yesterday's emails.",
    });
  });

  it("shows the server's invalid_schedule message inline under the schedule field", async () => {
    h.create.mockRejectedValue(
      new ApiError(
        400,
        "invalid_schedule",
        "invalid 5-field cron schedule (m h dom mon dow): expected exactly 5 fields"
      )
    );
    render(
      <RoutineFormDialog open onOpenChange={vi.fn()} botId="bot-1" />
    );

    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "Morning digest" },
    });
    fireEvent.change(screen.getByLabelText("Schedule"), {
      target: { value: "0 8 * *" },
    });
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Summarize yesterday's emails." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create routine" }));

    expect(
      await screen.findByText(
        "invalid 5-field cron schedule (m h dom mon dow): expected exactly 5 fields"
      )
    ).toBeInTheDocument();
    expect(h.create).toHaveBeenCalledTimes(1);
  });
});

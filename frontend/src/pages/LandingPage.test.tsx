import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LandingPage } from "./LandingPage";

describe("LandingPage", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders hero headline, pitch, and brand badge", () => {
    render(
      <MemoryRouter>
        <LandingPage />
      </MemoryRouter>
    );

    expect(screen.getByRole("heading", { level: 1, name: /The AI Workspace With a Soul/i })).toBeInTheDocument();
    expect(screen.getByText(/Tired of \$20\/month walled gardens\?/i)).toBeInTheDocument();
    expect(screen.getByText("Free & Open Source AI Workspace")).toBeInTheDocument();
  });

  it("explains why we call it Herbie and its Fantastic Four inspiration in the story section", () => {
    render(
      <MemoryRouter>
        <LandingPage />
      </MemoryRouter>
    );

    expect(screen.getByText(/Inspired by Fantastic Four’s Iconic Robot Ally/i)).toBeInTheDocument();
    expect(screen.getByText(/The Fantastic Four Inspiration \(H\.E\.R\.B\.I\.E\.\)/i)).toBeInTheDocument();
    expect(screen.getByText(/Mister Fantastic \(Reed Richards\) created/i)).toBeInTheDocument();
    expect(screen.getByText(/HarveyAvatar\.tsx/i)).toBeInTheDocument();
    expect(screen.getByText(/Occasional “Herbey” \/ “Harbey” Mix-Up/i)).toBeInTheDocument();
  });

  it("allows switching interactive mood controls for Herbie", async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <LandingPage />
      </MemoryRouter>
    );

    const jumpMoodBtn = screen.getByRole("button", { name: /Jump/i });
    expect(jumpMoodBtn).toBeInTheDocument();
    await user.click(jumpMoodBtn);

    const waveMoodBtn = screen.getByRole("button", { name: /Wave/i });
    await user.click(waveMoodBtn);
  });

  it("switches playground simulation tabs", async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <LandingPage />
      </MemoryRouter>
    );

    const sandboxTab = screen.getByRole("button", { name: /Code Sandbox/i });
    expect(sandboxTab).toBeInTheDocument();

    const mcpTab = screen.getByRole("button", { name: /MCP Ecosystem/i });
    await user.click(mcpTab);
    expect(screen.getByText(/Search GitHub PRs for 'streaming-rag'/i)).toBeInTheDocument();

    const workflowTab = screen.getByRole("button", { name: /Autonomous Agent Pipeline/i });
    await user.click(workflowTab);

    expect(screen.getByText(/Trigger the automated code review and release pipeline/i)).toBeInTheDocument();

    const ragTab = screen.getByRole("button", { name: /Project Document RAG/i });
    await user.click(ragTab);

    expect(screen.getByText(/How does Herbie structure relative parsed text storage/i)).toBeInTheDocument();
  });

  it("copies the docker compose quickstart command", async () => {
    const writeTextMock = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: {
        writeText: writeTextMock,
      },
      configurable: true,
    });

    render(
      <MemoryRouter>
        <LandingPage />
      </MemoryRouter>
    );

    const copyBtn = screen.getByTitle("Copy to clipboard");
    fireEvent.click(copyBtn);

    expect(writeTextMock).toHaveBeenCalledWith(
      "git clone https://github.com/abubakarsiddik31/herbie.git && cd herbie && docker compose up -d"
    );
  });
});

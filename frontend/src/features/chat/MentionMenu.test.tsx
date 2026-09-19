import { render, screen, fireEvent } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { MentionMenu } from "./MentionMenu";
import type { ChatApp } from "./useChatApps";

const MOCK_APPS: ChatApp[] = [
  {
    id: "google_calendar",
    name: "Google Calendar",
    mention: "calendar",
    description: "Check schedule and create events",
    type: "oauth",
    iconName: "calendar",
    connected: true,
    configured: true,
    requiresConnection: true,
  },
  {
    id: "github",
    name: "GitHub",
    mention: "github",
    description: "Search repos and manage issues",
    type: "oauth",
    iconName: "github",
    connected: false,
    configured: true,
    requiresConnection: true,
  },
];

describe("MentionMenu", () => {
  it("renders apps with name, mention, and connection badge", () => {
    render(
      <MentionMenu
        apps={MOCK_APPS}
        selectedIndex={0}
        onSelect={vi.fn()}
        onClose={vi.fn()}
      />,
    );

    expect(screen.getByText("Google Calendar")).toBeInTheDocument();
    expect(screen.getByText("@calendar")).toBeInTheDocument();
    expect(screen.getByText("Ready")).toBeInTheDocument();

    expect(screen.getByText("GitHub")).toBeInTheDocument();
    expect(screen.getByText("@github")).toBeInTheDocument();
    expect(screen.getByText("Connect")).toBeInTheDocument();
  });

  it("calls onSelect when an app option is clicked", () => {
    const handleSelect = vi.fn();
    render(
      <MentionMenu
        apps={MOCK_APPS}
        selectedIndex={1}
        onSelect={handleSelect}
        onClose={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByText("Google Calendar"));
    expect(handleSelect).toHaveBeenCalledWith(MOCK_APPS[0]);
  });

  it("displays empty message when apps list is empty", () => {
    render(
      <MentionMenu
        apps={[]}
        selectedIndex={0}
        onSelect={vi.fn()}
        onClose={vi.fn()}
      />,
    );

    expect(screen.getByText("No apps or tools matching query")).toBeInTheDocument();
  });
});

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { CommandPalette } from "./CommandPalette";

beforeAll(() => {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
});

afterEach(cleanup);

function renderPalette(props: Partial<Parameters<typeof CommandPalette>[0]> = {}) {
  const onNewChat = vi.fn();
  const onExport = vi.fn();
  const onShare = vi.fn();
  const onOpenSettings = vi.fn();
  const onOpenShortcuts = vi.fn();
  const onOpenChange = vi.fn();

  render(
    <MemoryRouter>
      <CommandPalette
        open={true}
        onOpenChange={onOpenChange}
        conversations={[
          {
            id: "c1",
            title: "Moon landing exploration",
            model: "gemini-2.5-flash",
            temperature: null,
            systemPrompt: "",
            ragEnabled: true,
            createdAt: "",
            updatedAt: "",
          },
        ]}
        activeConversation={{
          id: "c1",
          title: "Moon landing exploration",
          model: "gemini-2.5-flash",
          temperature: null,
          systemPrompt: "",
          ragEnabled: true,
          createdAt: "",
          updatedAt: "",
        }}
        onNewChat={onNewChat}
        onExport={onExport}
        onShare={onShare}
        onOpenSettings={onOpenSettings}
        onOpenShortcuts={onOpenShortcuts}
        {...props}
      />
    </MemoryRouter>,
  );

  return { onNewChat, onExport, onShare, onOpenSettings, onOpenShortcuts, onOpenChange };
}

describe("CommandPalette", () => {
  it("renders actions and recent conversations", () => {
    renderPalette();
    expect(screen.getByPlaceholderText(/type a command/i)).toBeInTheDocument();
    expect(screen.getByText("New chat")).toBeInTheDocument();
    expect(screen.getByText("Moon landing exploration")).toBeInTheDocument();
  });

  it("filters items by search query", async () => {
    renderPalette();
    const input = screen.getByPlaceholderText(/type a command/i);
    await userEvent.type(input, "landing");
    expect(screen.getByText("Moon landing exploration")).toBeInTheDocument();
    expect(screen.queryByText("New chat")).not.toBeInTheDocument();
  });

  it("triggers action on click", async () => {
    const { onNewChat, onOpenChange } = renderPalette();
    await userEvent.click(screen.getByText("New chat"));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(onNewChat).toHaveBeenCalled();
  });

  it("navigates with keyboard and runs with Enter", async () => {
    const { onOpenChange } = renderPalette();
    const input = screen.getByPlaceholderText(/type a command/i);
    await userEvent.type(input, "documents");
    await userEvent.keyboard("{Enter}");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});

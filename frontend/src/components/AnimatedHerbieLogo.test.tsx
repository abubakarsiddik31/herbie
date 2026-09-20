import { render, screen, fireEvent } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AnimatedHerbieLogo } from "./AnimatedHerbieLogo";

describe("AnimatedHerbieLogo", () => {
  it("renders mascot svg with default float mood", () => {
    render(<AnimatedHerbieLogo />);
    const mascot = screen.getByRole("button", { name: "Herbie the bubble-bot mascot" });
    expect(mascot).toBeInTheDocument();
  });

  it("displays custom speech bubble text when provided", () => {
    render(<AnimatedHerbieLogo bubbleText="Hello from Herbie!" />);
    expect(screen.getByText("Hello from Herbie!")).toBeInTheDocument();
  });

  it("cycles quotes and triggers onClick callback when clicked in interactive mode", () => {
    const handleClick = vi.fn();
    render(<AnimatedHerbieLogo onClick={handleClick} interactive={true} />);

    const button = screen.getByRole("button", { name: "Herbie the bubble-bot mascot" });
    fireEvent.click(button);

    expect(handleClick).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("status")).toBeInTheDocument();
  });

  it("renders non-interactive image role when interactive is false", () => {
    render(<AnimatedHerbieLogo interactive={false} />);
    expect(screen.getByRole("img", { name: "Herbie the bubble-bot mascot" })).toBeInTheDocument();
  });

  it("renders with wordmark when showWordmark is true", () => {
    render(<AnimatedHerbieLogo showWordmark={true} />);
    expect(screen.getByText("The Free AI Workspace")).toBeInTheDocument();
  });
});

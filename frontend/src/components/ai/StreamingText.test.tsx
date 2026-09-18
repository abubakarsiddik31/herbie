import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { StreamingText } from "./StreamingText";

describe("StreamingText citations", () => {
  it("links a bracket citation to its source number", async () => {
    const onCite = vi.fn();
    const user = userEvent.setup();
    render(<StreamingText content="Herons nest here [2]." citations={{ count: 3, onCite }} />);
    const link = screen.getByRole("button", { name: "View source 2" });
    expect(link).toHaveTextContent("2");
    await user.click(link);
    expect(onCite).toHaveBeenCalledWith(2);
  });

  it("links each number in a grouped citation", () => {
    const onCite = vi.fn();
    render(<StreamingText content="Both agree [1, 3]." citations={{ count: 3, onCite }} />);
    expect(screen.getByRole("button", { name: "View source 1" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "View source 3" })).toBeInTheDocument();
  });

  it("leaves out-of-range citations as plain text", () => {
    const onCite = vi.fn();
    const { container } = render(
      <StreamingText content="Mystery claim [9]." citations={{ count: 2, onCite }} />,
    );
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(container).toHaveTextContent("Mystery claim [9].");
  });

  it("does not link citations inside code spans", () => {
    const onCite = vi.fn();
    render(<StreamingText content="Run `[1]` to test." citations={{ count: 3, onCite }} />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("renders plain text without the citations prop", () => {
    const { container } = render(<StreamingText content="Plain [1] text." />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(container).toHaveTextContent("Plain [1] text.");
  });

  it("leaves normal markdown links alone", () => {
    const onCite = vi.fn();
    render(
      <StreamingText content="See [docs](https://example.com) [1]." citations={{ count: 2, onCite }} />,
    );
    expect(screen.getByRole("link", { name: "docs" })).toHaveAttribute("href", "https://example.com");
    expect(screen.getByRole("button", { name: "View source 1" })).toBeInTheDocument();
  });
});

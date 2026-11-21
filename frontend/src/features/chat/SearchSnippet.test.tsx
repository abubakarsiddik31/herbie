import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { HighlightedSnippet, parseSnippet } from "./SearchSnippet";

describe("SearchSnippet", () => {
  it("splits sentinel markers into hit and plain parts", () => {
    expect(parseSnippet("before \uE000match\uE001 after")).toEqual([
      { text: "before ", hit: false },
      { text: "match", hit: true },
      { text: " after", hit: false },
    ]);
  });

  it("renders matches as mark text nodes, not HTML", () => {
    const { container } = render(
      <HighlightedSnippet snippet={"\uE000<img src=x>\uE001"} />,
    );
    const mark = container.querySelector("mark");
    expect(mark).not.toBeNull();
    expect(mark?.textContent).toBe("<img src=x>");
    expect(container.querySelector("img")).toBeNull();
    expect(screen.getByText("<img src=x>")).toBeInTheDocument();
  });
});

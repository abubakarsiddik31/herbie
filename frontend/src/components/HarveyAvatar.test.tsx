import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { HarveyAvatar } from "./HarveyAvatar";

describe("HarveyAvatar", () => {
  it("renders without background color/box and has accessible attributes", () => {
    const { container } = render(<HarveyAvatar />);

    // Must NOT contain solid background box classes like bg-brand or rounded square boxes
    const avatarWrapper = container.firstChild as HTMLElement;
    expect(avatarWrapper.className).not.toContain("bg-brand");
    expect(avatarWrapper.className).not.toContain("bg-card");

    // Accessible role and default aria label
    expect(screen.getByRole("img", { name: "Harvey avatar" })).toBeInTheDocument();
  });

  it("applies jumping animation and processing label when isProcessing is true", () => {
    const { container } = render(<HarveyAvatar isProcessing={true} />);

    // Accessible label indicates active processing
    expect(screen.getByRole("img", { name: "Harvey is thinking and processing..." })).toBeInTheDocument();

    // SVG character group has the jumping animation class
    const jumpingGroup = container.querySelector(".animate-harvey-jump");
    expect(jumpingGroup).toBeInTheDocument();

    // Floor shadow has the shadow pulse animation class
    const shadowEllipse = container.querySelector(".animate-harvey-shadow");
    expect(shadowEllipse).toBeInTheDocument();
  });

  it("does not apply jumping animation when idle", () => {
    const { container } = render(<HarveyAvatar isProcessing={false} />);

    const jumpingGroup = container.querySelector(".animate-harvey-jump");
    expect(jumpingGroup).not.toBeInTheDocument();

    const shadowEllipse = container.querySelector(".animate-harvey-shadow");
    expect(shadowEllipse).not.toBeInTheDocument();
  });
});

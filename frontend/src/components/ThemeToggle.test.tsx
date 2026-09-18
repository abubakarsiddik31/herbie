import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider } from "next-themes";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { ThemeToggle } from "./ThemeToggle";

beforeAll(() => {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  });
});

afterEach(() => {
  cleanup();
  localStorage.clear();
  document.documentElement.classList.remove("dark");
});

function renderToggle(initial = "light") {
  localStorage.setItem("theme", initial);
  return render(
    <ThemeProvider attribute="class" defaultTheme="system" enableSystem>
      <ThemeToggle />
    </ThemeProvider>,
  );
}

describe("ThemeToggle", () => {
  it("cycles light to dark and applies the dark class", async () => {
    renderToggle("light");
    const button = await screen.findByRole("button", { name: /theme: light/i });
    await userEvent.click(button);
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(localStorage.getItem("theme")).toBe("dark");
    expect(await screen.findByRole("button", { name: /theme: dark/i })).toBeInTheDocument();
  });

  it("cycles dark to system and clears the forced class", async () => {
    renderToggle("dark");
    const button = await screen.findByRole("button", { name: /theme: dark/i });
    await userEvent.click(button);
    expect(document.documentElement.classList.contains("dark")).toBe(false);
    expect(localStorage.getItem("theme")).toBe("system");
  });
});

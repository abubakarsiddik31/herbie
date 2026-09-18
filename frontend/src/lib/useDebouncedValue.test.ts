import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useDebouncedValue } from "./useDebouncedValue";

describe("useDebouncedValue", () => {
  it("delays updates until the value settles", () => {
    vi.useFakeTimers();
    try {
      const { result, rerender } = renderHook(({ value }) => useDebouncedValue(value, 250), {
        initialProps: { value: "a" },
      });
      expect(result.current).toBe("a");
      rerender({ value: "ab" });
      act(() => {
        vi.advanceTimersByTime(100);
      });
      expect(result.current).toBe("a");
      rerender({ value: "abc" });
      act(() => {
        vi.advanceTimersByTime(250);
      });
      expect(result.current).toBe("abc");
    } finally {
      vi.useRealTimers();
    }
  });
});

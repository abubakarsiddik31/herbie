import { describe, expect, it } from "vitest";
import { fmtTokens } from "./utils";

describe("fmtTokens", () => {
  it("renders small counts as-is", () => {
    expect(fmtTokens(0)).toBe("0");
    expect(fmtTokens(999)).toBe("999");
  });
  it("renders thousands compactly", () => {
    expect(fmtTokens(1200)).toBe("1.2k");
    expect(fmtTokens(45600)).toBe("45.6k");
  });
});

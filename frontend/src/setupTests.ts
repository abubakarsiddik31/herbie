import "@testing-library/jest-dom/vitest";

if (typeof window !== "undefined") {
  const svgProto = (window.SVGElement?.prototype ?? {}) as unknown as { getBBox?: () => unknown };
  if (!svgProto.getBBox) {
    svgProto.getBBox = () => ({
      x: 0,
      y: 0,
      width: 100,
      height: 100,
      top: 0,
      left: 0,
      right: 100,
      bottom: 100,
      toJSON: () => "",
    });
  }
}



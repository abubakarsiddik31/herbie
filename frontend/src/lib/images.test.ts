import { describe, expect, it } from "vitest";
import { ALLOWED_IMAGE_TYPES, MAX_IMAGE_BYTES, readImageFiles } from "./images";

function makeFile(type: string, size: number, name = "pic"): File {
  const content = new Uint8Array(size);
  return new File([content], name, { type });
}

describe("readImageFiles", () => {
  it("loads a valid png into wire format", async () => {
    const { images, errors } = await readImageFiles([makeFile("image/png", 16, "tiny.png")]);
    expect(errors).toEqual([]);
    expect(images).toHaveLength(1);
    expect(images[0].mediaType).toBe("image/png");
    expect(images[0].data).not.toContain(",");
    expect(images[0].previewUrl).toMatch(/^blob:|data:/);
  });

  it("rejects unsupported types and oversized files", async () => {
    const { images, errors } = await readImageFiles([
      makeFile("image/bmp", 10, "weird.bmp"),
      makeFile("image/png", MAX_IMAGE_BYTES + 1, "huge.png"),
    ]);
    expect(images).toEqual([]);
    expect(errors).toHaveLength(2);
  });

  it("caps the number of images per message", async () => {
    const files = ALLOWED_IMAGE_TYPES.map((t) => makeFile(t, 8)).concat(makeFile("image/png", 8, "extra"));
    const { images, errors } = await readImageFiles(files);
    expect(images).toHaveLength(4);
    expect(errors).toEqual(["At most 4 images per message."]);
  });
});

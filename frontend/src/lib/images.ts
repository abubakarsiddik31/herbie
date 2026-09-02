// Client-side mirror of the backend's image attachment limits (see
// backend/internal/httpapi/images.go) — reject early, before upload.

export const MAX_IMAGES_PER_MESSAGE = 4;
export const MAX_IMAGE_BYTES = 4 << 20; // per decoded image

export const ALLOWED_IMAGE_TYPES = ["image/png", "image/jpeg", "image/webp", "image/gif"] as const;

export interface PendingImage {
  mediaType: string;
  /** Standard base64, no data-URL prefix — the wire format. */
  data: string;
  /** Object URL for the composer preview. */
  previewUrl: string;
  name: string;
}

// readImageFiles validates and decodes picked image files. It stops at the
// per-message cap and reports every problem file; valid files still load.
export async function readImageFiles(files: File[]): Promise<{ images: PendingImage[]; errors: string[] }> {
  const images: PendingImage[] = [];
  const errors: string[] = [];
  for (const file of files) {
    if (images.length >= MAX_IMAGES_PER_MESSAGE) {
      errors.push(`At most ${MAX_IMAGES_PER_MESSAGE} images per message.`);
      break;
    }
    if (!(ALLOWED_IMAGE_TYPES as readonly string[]).includes(file.type)) {
      errors.push(`${file.name}: unsupported type.`);
      continue;
    }
    if (file.size > MAX_IMAGE_BYTES) {
      errors.push(`${file.name}: larger than 4 MB.`);
      continue;
    }
    try {
      const dataUrl = await readFileAsDataURL(file);
      images.push({
        mediaType: file.type,
        data: dataUrl.slice(dataUrl.indexOf(",") + 1),
        previewUrl: URL.createObjectURL(file),
        name: file.name,
      });
    } catch {
      errors.push(`${file.name}: could not be read.`);
    }
  }
  return { images, errors };
}

function readFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

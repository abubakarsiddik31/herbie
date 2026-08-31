import { describe, expect, it } from "vitest";
import { parseSSE } from "./sse";

function sseResponse(frames: string): Response {
  const stream = new ReadableStream({
    start(controller) {
      controller.enqueue(new TextEncoder().encode(frames));
      controller.close();
    },
  });
  return new Response(stream, { headers: { "Content-Type": "text/event-stream" } });
}

describe("parseSSE", () => {
  it("yields named events with data payloads", async () => {
    const res = sseResponse('event: delta\ndata: {"text":"Hi"}\n\nevent: done\ndata: {"requests":1}\n\n');
    const events: { event: string; data: string }[] = [];
    for await (const e of parseSSE(res)) events.push(e);
    expect(events).toEqual([
      { event: "delta", data: '{"text":"Hi"}' },
      { event: "done", data: '{"requests":1}' },
    ]);
  });
});

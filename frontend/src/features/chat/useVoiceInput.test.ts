import { act } from "react";
import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useVoiceInput, type RecognitionCtor, type RecognitionLike } from "./useVoiceInput";

// scriptedRecognition is a controllable fake recognizer: tests emit result
// events and end events through the captured handlers.
class ScriptedRecognition implements RecognitionLike {
  lang = "";
  continuous = false;
  interimResults = false;
  started = false;
  onresult: ((e: any) => void) | null = null;
  onend: (() => void) | null = null;
  onerror: ((e: { error: string }) => void) | null = null;

  start() { this.started = true; }
  stop() { this.started = false; this.onend?.(); }
  abort() { this.started = false; this.onend?.(); }

  emit(finals: string[], interim: string) {
    this.onresult?.({
      resultIndex: 0,
      results: [
        ...finals.map((t) => ({ isFinal: true, 0: { transcript: t } })),
        { isFinal: false, 0: { transcript: interim } },
      ],
    });
  }
}

const originalWindow = window;

afterEach(() => {
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
});

function stubCtor(ctor: RecognitionCtor | null) {
  vi.stubGlobal("window", Object.defineProperties(originalWindow, {
    SpeechRecognition: { value: ctor, configurable: true },
    webkitSpeechRecognition: { value: undefined, configurable: true },
  }));
}

describe("useVoiceInput", () => {
  it("reports unsupported when no recognition API exists", () => {
    stubCtor(null);
    const { result } = renderHook(() => useVoiceInput({ onInterim: vi.fn(), onFinal: vi.fn() }));
    expect(result.current.supported).toBe(false);
    expect(result.current.listening).toBe(false);
  });

  it("streams interim and final transcripts while listening", () => {
    const onInterim = vi.fn();
    const onFinal = vi.fn();
    let instance: ScriptedRecognition;
    const Ctor = class implements RecognitionLike {
      constructor() {
        instance = new ScriptedRecognition();
        return instance;
      }
      lang = ""; continuous = false; interimResults = false;
      start() { instance.started = true; }
      stop() { instance.onend?.(); }
      abort() { instance.onend?.(); }
      onresult: ((e: any) => void) | null = null;
      onend: (() => void) | null = null;
      onerror: ((e: { error: string }) => void) | null = null;
    } as unknown as RecognitionCtor;
    stubCtor(Ctor);
    vi.stubGlobal("navigator", { ...navigator, language: "de-DE" });

    const { result } = renderHook(() => useVoiceInput({ onInterim, onFinal }));
    expect(result.current.supported).toBe(true);

    act(() => result.current.toggle());
    expect(result.current.listening).toBe(true);

    act(() => instance!.emit(["hello there"], "wor"));
    expect(onFinal).toHaveBeenCalledWith("hello there");
    expect(onInterim).toHaveBeenCalledWith("wor");

    act(() => result.current.toggle());
    expect(result.current.listening).toBe(false);
  });
});

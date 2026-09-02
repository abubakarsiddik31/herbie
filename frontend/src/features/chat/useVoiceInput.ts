import { useCallback, useEffect, useRef, useState } from "react";

// Minimal structural typings for the Web Speech API, which TS's DOM lib
// only covers partially across targets.
interface SpeechRecognitionAlternative { transcript: string }
interface SpeechRecognitionResultLike { isFinal: boolean; 0: SpeechRecognitionAlternative }
interface SpeechRecognitionEventLike {
  resultIndex: number;
  results: { length: number; [index: number]: SpeechRecognitionResultLike };
}
export interface RecognitionLike {
  lang: string;
  continuous: boolean;
  interimResults: boolean;
  start(): void;
  stop(): void;
  abort(): void;
  onresult: ((e: SpeechRecognitionEventLike) => void) | null;
  onend: (() => void) | null;
  onerror: ((e: { error: string }) => void) | null;
}
export type RecognitionCtor = new () => RecognitionLike;

function getRecognitionCtor(): RecognitionCtor | null {
  const w = window as unknown as { SpeechRecognition?: RecognitionCtor; webkitSpeechRecognition?: RecognitionCtor };
  return w.SpeechRecognition ?? w.webkitSpeechRecognition ?? null;
}

interface Options {
  /** Fires on every interim (not yet finalized) transcript fragment. */
  onInterim: (text: string) => void;
  /** Fires once per finalized phrase. */
  onFinal: (text: string) => void;
}

// useVoiceInput wraps browser speech recognition into dictation events.
// Nothing is recorded or stored — transcripts land straight in the caller's
// state. Callbacks are held in refs so the recognition instance survives
// re-renders.
export function useVoiceInput({ onInterim, onFinal }: Options) {
  const [listening, setListening] = useState(false);
  const recRef = useRef<RecognitionLike | null>(null);
  const interimRef = useRef(onInterim);
  const finalRef = useRef(onFinal);
  useEffect(() => {
    interimRef.current = onInterim;
    finalRef.current = onFinal;
  }, [onInterim, onFinal]);

  const stop = useCallback(() => {
    recRef.current?.stop();
    recRef.current = null;
    setListening(false);
  }, []);

  const start = useCallback(() => {
    const Ctor = getRecognitionCtor();
    if (!Ctor) return;
    const rec = new Ctor();
    rec.lang = navigator.language || "en-US";
    rec.continuous = true;
    rec.interimResults = true;
    rec.onresult = (e) => {
      let interim = "";
      for (let i = e.resultIndex; i < e.results.length; i++) {
        const result = e.results[i];
        if (result.isFinal) finalRef.current(result[0].transcript.trim());
        else interim += result[0].transcript;
      }
      if (interim) interimRef.current(interim.trim());
    };
    rec.onend = () => setListening(false);
    rec.onerror = () => setListening(false);
    recRef.current = rec;
    rec.start();
    setListening(true);
  }, []);

  const toggle = useCallback(() => {
    if (listening) stop();
    else start();
  }, [listening, start, stop]);

  // Abort on unmount so a live recognizer doesn't outlive the page.
  useEffect(() => () => recRef.current?.abort(), []);

  return { supported: getRecognitionCtor() !== null, listening, toggle };
}

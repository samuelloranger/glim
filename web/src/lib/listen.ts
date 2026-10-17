import type { ListenStatus } from "./types";

const STEPS: Record<string, string> = {
  queued: "Waiting for another audio version to finish…",
  page: "Reading the page…",
  script: "Writing the narration…",
  audio: "Recording the audio…",
};

/** What a working job is doing, in words. */
export function stepText(s: Pick<ListenStatus, "step" | "done" | "total">): string {
  if (s.step === "audio" && (s.total ?? 0) > 1) {
    return `Recording the audio, part ${Math.min((s.done ?? 0) + 1, s.total ?? 1)} of ${s.total}…`;
  }
  return STEPS[s.step ?? ""] ?? "Working…";
}

/** A duration in seconds as m:ss. */
export function clockTime(sec: number): string {
  let m = Math.floor(sec / 60);
  let s = Math.round(sec % 60);
  if (s === 60) {
    m++;
    s = 0;
  }
  return `${m}:${s < 10 ? "0" : ""}${s}`;
}

export const RATES = [1, 1.25, 1.5, 2] as const;

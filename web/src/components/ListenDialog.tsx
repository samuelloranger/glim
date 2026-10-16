import { createEffect, createMemo, createSignal, For, Show } from "solid-js";
import { api, errorText } from "../lib/api";
import { clockTime, RATES, stepText } from "../lib/listen";
import { listenAudioSrc } from "../lib/preview";
import { formatAgo } from "../lib/time";
import type { ListenResult, ListenStatus, Preview } from "../lib/types";

const POLL_MS = 2000;

/**
 * The audio version of a preview: make it, follow its progress, then play it
 * with its summary and transcript. The player outlives the dialog, so audio
 * keeps playing after it closes, with a small bar to pause it or reopen.
 */
export function ListenDialog(props: {
  preview: Preview | null;
  title: string;
  onClose: () => void;
  onReopen: (name: string) => void;
}) {
  let dialog: HTMLDialogElement | undefined;
  let audio: HTMLAudioElement | undefined;
  const [status, setStatus] = createSignal<ListenStatus | null>(null);
  const [error, setError] = createSignal("");
  // What the player holds: set when a preview's audio is loaded into it.
  const [loaded, setLoaded] = createSignal<{ name: string; title: string; key: string } | null>(
    null,
  );
  const [playing, setPlaying] = createSignal(false);
  // Started and not finished: the mini player stays while paused.
  const [active, setActive] = createSignal(false);
  const [rate, setRate] = createSignal(1);
  const [speaking, setSpeaking] = createSignal(false);
  let timer: ReturnType<typeof setTimeout> | undefined;
  let gen = 0;

  function stopPoll() {
    clearTimeout(timer);
    timer = undefined;
  }

  async function refresh(name: string, mine: number) {
    try {
      const s = await api.listen(name);
      if (mine !== gen) return;
      setStatus(s);
      setError("");
      if (s.state === "working") timer = setTimeout(() => void refresh(name, mine), POLL_MS);
    } catch (e) {
      if (mine === gen) setError(errorText(e));
    }
  }

  createEffect(
    () => props.preview?.name ?? "",
    (name) => {
      if (!dialog) return;
      gen++;
      stopPoll();
      if (name) {
        setStatus(null);
        setError("");
        if (!dialog.open) dialog.showModal();
        void refresh(name, gen);
      } else {
        // Forget this preview's status, so opening another one never pairs
        // it with this one's result.
        setStatus(null);
        if (dialog.open) dialog.close();
      }
    },
  );

  async function start(again: boolean) {
    const p = props.preview;
    if (!p) return;
    const mine = gen;
    stopPoll();
    setError("");
    setStatus((s) => ({
      state: "working",
      step: "queued",
      stale: false,
      speech: s?.speech ?? true,
      result: s?.result,
    }));
    try {
      const s = await api.startListen(p.name, again);
      if (mine !== gen) return;
      setStatus(s);
      if (s.state === "working") timer = setTimeout(() => void refresh(p.name, mine), POLL_MS);
    } catch (e) {
      if (mine !== gen) return;
      setError(errorText(e));
      void refresh(p.name, mine);
    }
  }

  const result = createMemo(() => status()?.result);
  const key = (r: ListenResult) => `${r.version}:${r.created}`;
  // Point the player at the open preview's audio, unless it already holds it.
  createEffect(
    () => {
      const p = props.preview;
      const r = result();
      return p && r?.audio ? { p, r } : null;
    },
    (cur) => {
      if (!cur || !audio) return;
      const k = key(cur.r);
      const l = loaded();
      if (l && l.name === cur.p.name && l.key === k) return;
      audio.src = listenAudioSrc(cur.p, cur.r);
      setActive(false);
      audio.playbackRate = rate();
      setLoaded({ name: cur.p.name, title: props.title, key: k });
      try {
        if ("mediaSession" in navigator) {
          navigator.mediaSession.metadata = new MediaMetadata({
            title: props.title,
            artist: "glim",
          });
        }
      } catch {
        // No media session (older browsers): the lock screen shows no title.
      }
    },
  );

  const showsPlayer = createMemo(() => {
    const p = props.preview;
    const r = result();
    const l = loaded();
    return !!(p && r?.audio && l && l.name === p.name && l.key === key(r));
  });

  function pickRate(v: number) {
    setRate(v);
    if (audio) audio.playbackRate = v;
  }

  function speak(r: ListenResult) {
    const ss = window.speechSynthesis;
    if (speaking()) {
      ss.cancel();
      setSpeaking(false);
      return;
    }
    const u = new SpeechSynthesisUtterance(r.narration);
    if (r.language) u.lang = r.language;
    u.onend = () => setSpeaking(false);
    u.onerror = () => setSpeaking(false);
    setSpeaking(true);
    ss.speak(u);
  }

  const canSpeak = typeof window !== "undefined" && "speechSynthesis" in window;
  const meta = (r: ListenResult) => {
    const parts: string[] = [];
    if (r.seconds) parts.push(clockTime(r.seconds));
    parts.push(formatAgo(Date.now() - Date.parse(r.created)));
    if (r.model) parts.push(r.model);
    if (!r.rendered) parts.push("read from the HTML, without screenshots");
    return parts.join(" · ");
  };

  return (
    <>
      <dialog
        ref={(el) => (dialog = el)}
        class="dialog listen"
        aria-labelledby="listen-title"
        onClose={() => {
          if (props.preview) props.onClose();
        }}
      >
        <h2 id="listen-title">Listen to {props.title}</h2>
        {/* Outside the Show blocks so it survives closing the dialog. */}
        <div class="listen-player" hidden={!showsPlayer()}>
          {/* biome-ignore lint/a11y/useMediaCaption: the transcript is shown below the player. */}
          <audio
            ref={(el) => (audio = el)}
            controls
            preload="metadata"
            aria-label="Audio version"
            onPlay={() => {
              setPlaying(true);
              setActive(true);
            }}
            onPause={() => setPlaying(false)}
            onEnded={() => {
              setPlaying(false);
              setActive(false);
            }}
          />
          <fieldset class="listen-rates">
            <legend class="visually-hidden">Playback speed</legend>
            <For each={RATES}>
              {(v) => (
                <button
                  class={["btn quiet", { on: rate() === v }]}
                  type="button"
                  aria-pressed={rate() === v ? "true" : "false"}
                  onClick={() => pickRate(v)}
                >
                  {v}×
                </button>
              )}
            </For>
          </fieldset>
        </div>
        <Show when={props.preview}>
          <Show when={status()} fallback={<p class="help">Loading…</p>}>
            {(s) => (
              <div class="listen-body">
                <Show when={s().state === "working"}>
                  <p class="listen-step" role="status">
                    {stepText(s())}
                  </p>
                  <p class="help">You can close this; it keeps going.</p>
                </Show>
                <Show when={s().state === "failed"}>
                  <p class="error" role="alert">
                    {s().error || "It didn't work."}
                  </p>
                </Show>
                <Show when={s().state === "none"}>
                  <p class="help">
                    Get a spoken walkthrough of this page: a model describes its text, pictures and
                    design, and it's read aloud.
                  </p>
                </Show>
                <Show when={s().stale && s().state !== "working"}>
                  <p class="help">The page changed since this was made.</p>
                </Show>
                <Show when={s().state !== "working" ? s().result : undefined}>
                  {(r) => (
                    <>
                      <Show when={!r().audio && canSpeak}>
                        <button class="btn" type="button" onClick={() => speak(r())}>
                          {speaking() ? "Stop" : "Read aloud"}
                        </button>
                      </Show>
                      <Show when={r().truncated}>
                        <p class="help">
                          The model stopped before the end of the page, so the audio ends early.
                        </p>
                      </Show>
                      <p class="listen-summary">{r().summary}</p>
                      <p class="help">{meta(r())}</p>
                      <details class="listen-transcript">
                        <summary>Transcript</summary>
                        <p>{r().narration}</p>
                      </details>
                    </>
                  )}
                </Show>
              </div>
            )}
          </Show>
        </Show>

        <Show when={error()}>
          <p class="error" role="alert">
            {error()}
          </p>
        </Show>
        <div class="dialog-actions">
          <button class="btn" type="button" onClick={() => props.onClose()}>
            Close
          </button>
          <Show when={status()?.state === "none"}>
            <button class="btn primary" type="button" onClick={() => void start(false)}>
              Make the audio version
            </button>
          </Show>
          <Show when={status()?.state === "failed"}>
            <button class="btn primary" type="button" onClick={() => void start(true)}>
              Try again
            </button>
          </Show>
          <Show when={status()?.state === "ready"}>
            <button
              class={["btn", { primary: status()?.stale === true }]}
              type="button"
              onClick={() => void start(true)}
            >
              {status()?.stale ? "Update" : "Make it again"}
            </button>
          </Show>
        </div>
      </dialog>
      <Show when={!props.preview && active() ? loaded() : undefined}>
        {(l) => (
          <section class="now-playing" aria-label="Now playing">
            <button
              class="btn quiet"
              type="button"
              onClick={() => (playing() ? audio?.pause() : void audio?.play())}
            >
              {playing() ? "Pause" : "Play"}
            </button>
            <button class="now-title" type="button" onClick={() => props.onReopen(l().name)}>
              {l().title}
            </button>
          </section>
        )}
      </Show>
    </>
  );
}

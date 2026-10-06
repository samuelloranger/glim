import { createSignal, createStore, reconcile } from "solid-js";
import { CLOSE_MS } from "./actions";
import { type Api, ApiError } from "./api";
import { skewMs } from "./time";
import type { Preview, Snapshot, Status } from "./types";

export type Conn = "connecting" | "live" | "reconnecting";

type LiveState = { loaded: boolean; previews: Preview[]; status: Status };

/** A tab hidden for less than this keeps its stream; longer, it is replaced. */
export const STALE_HIDDEN_MS = 30_000;

export type LiveDeps = {
  api: Pick<Api, "previews" | "session">;
  onUnauthorized: () => void;
  EventSourceImpl?: typeof EventSource;
  retryMs?: number;
  now?: () => number;
  /** How long a server-removed card stays (closing) before it is dropped. */
  closeMs?: number;
  staleHiddenMs?: number;
};

export function createLive(deps: LiveDeps) {
  const ES = deps.EventSourceImpl ?? EventSource;
  const now = deps.now ?? Date.now;
  const retryMs = deps.retryMs ?? 2000;
  const closeMs = deps.closeMs ?? CLOSE_MS;
  const staleHiddenMs = deps.staleHiddenMs ?? STALE_HIDDEN_MS;
  const [state, setState] = createStore<LiveState>({
    loaded: false,
    previews: [],
    status: { live: 0, pinned: 0, diskBytes: 0, nextExpiry: null },
  });
  // Names the server removed that are still on screen playing their close animation.
  const [leaving, setLeaving] = createStore<Record<string, boolean>>({});
  const [conn, setConn] = createSignal<Conn>("connecting");
  const [skew, setSkew] = createSignal(0);
  let source: EventSource | null = null;
  let retry: ReturnType<typeof setTimeout> | undefined;
  let stopped = true;

  // Cards the server dropped are kept for closeMs, flagged in `leaving`, so the
  // close animation plays in full instead of the card vanishing.
  function withDeparted(snap: Snapshot): Preview[] {
    if (!state.loaded) return snap.previews;
    const incoming = new Set(snap.previews.map((p) => p.name));
    const merged = [...snap.previews];
    state.previews.forEach((old, i) => {
      if (incoming.has(old.name)) {
        if (leaving[old.name]) setLeaving((d) => void (d[old.name] = false));
        return;
      }
      merged.splice(Math.min(i, merged.length), 0, { ...old });
      if (leaving[old.name]) return;
      const name = old.name;
      setLeaving((d) => void (d[name] = true));
      setTimeout(() => {
        if (!leaving[name]) return;
        setLeaving((d) => void (d[name] = false));
        drop(name);
      }, closeMs);
    });
    return merged;
  }

  function apply(snap: Snapshot) {
    const previews = withDeparted(snap);
    setState((d) => {
      reconcile(previews, "name")(d.previews);
      d.status = snap.status;
      d.loaded = true;
    });
    setSkew(skewMs(snap.now, now()));
  }

  function patch(p: Preview) {
    setState((d) => {
      const item = d.previews.find((x) => x.name === p.name);
      if (item) Object.assign(item, p);
    });
  }

  function drop(name: string) {
    setState((d) => {
      const i = d.previews.findIndex((x) => x.name === name);
      if (i >= 0) d.previews.splice(i, 1);
    });
  }

  async function refresh() {
    try {
      apply(await deps.api.previews());
    } catch {
      // The stream (or the next refresh) recovers; a 401 already signed us out.
    }
  }

  function open() {
    if (stopped) return;
    const es = new ES("/_glim/api/events");
    source = es;
    es.addEventListener("snapshot", (e) => {
      apply(JSON.parse((e as MessageEvent<string>).data) as Snapshot);
      setConn("live");
    });
    es.onerror = () => {
      setConn("reconnecting");
      if (es.readyState !== ES.CLOSED) return; // the browser retries on its own
      es.close();
      // Only the chain for the current stream may reopen: stop()/resume() while
      // this check is in flight has already replaced or retired `es`.
      const current = () => !stopped && source === es;
      deps.api.session().then(
        () => {
          if (current()) retry = setTimeout(open, retryMs);
        },
        (err: unknown) => {
          if (!current()) return;
          if (err instanceof ApiError && err.status === 401) deps.onUnauthorized();
          else retry = setTimeout(open, retryMs);
        },
      );
    };
  }

  function stop() {
    stopped = true;
    clearTimeout(retry);
    source?.close();
    source = null;
  }

  function start() {
    stopped = false;
    open();
    return stop;
  }

  // After sleep a mobile browser can keep a dead socket in OPEN state without
  // firing "error". Replace the stream outright; its first snapshot is the refresh.
  function resume() {
    stop();
    setConn("reconnecting");
    start();
  }

  // Coming back to a tab hidden only briefly keeps its stream; after longer a
  // mobile browser may have a dead socket, so replace it.
  let hiddenAt: number | null = null;
  function hide() {
    if (hiddenAt === null) hiddenAt = now();
  }
  function show() {
    const at = hiddenAt;
    hiddenAt = null;
    if (at !== null && now() - at > staleHiddenMs) resume();
  }

  return {
    state,
    leaving,
    conn,
    skew,
    apply,
    patch,
    drop,
    refresh,
    start,
    stop,
    resume,
    hide,
    show,
  };
}

export type Live = ReturnType<typeof createLive>;

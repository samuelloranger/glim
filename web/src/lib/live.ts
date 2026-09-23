import { createSignal, createStore, reconcile } from "solid-js";
import { type Api, ApiError } from "./api";
import { skewMs } from "./time";
import type { Preview, Snapshot, Status } from "./types";

export type Conn = "connecting" | "live" | "reconnecting";

type LiveState = { loaded: boolean; previews: Preview[]; status: Status };

export type LiveDeps = {
  api: Pick<Api, "previews" | "session">;
  onUnauthorized: () => void;
  EventSourceImpl?: typeof EventSource;
  retryMs?: number;
  now?: () => number;
};

export function createLive(deps: LiveDeps) {
  const ES = deps.EventSourceImpl ?? EventSource;
  const now = deps.now ?? Date.now;
  const retryMs = deps.retryMs ?? 2000;
  const [state, setState] = createStore<LiveState>({
    loaded: false,
    previews: [],
    status: { live: 0, pinned: 0, diskBytes: 0, nextExpiry: null },
  });
  const [conn, setConn] = createSignal<Conn>("connecting");
  const [skew, setSkew] = createSignal(0);
  let source: EventSource | null = null;
  let retry: ReturnType<typeof setTimeout> | undefined;
  let stopped = true;

  function apply(snap: Snapshot) {
    setState((d) => {
      reconcile(snap.previews, "name")(d.previews);
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

  return { state, conn, skew, apply, patch, drop, refresh, start, stop, resume };
}

export type Live = ReturnType<typeof createLive>;

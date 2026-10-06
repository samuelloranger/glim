import { expect, test } from "bun:test";
import { createRoot, flush } from "solid-js";
import { createLive } from "./live";
import type { Preview, Snapshot } from "./types";

class FakeES {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSED = 2;
  static instances: FakeES[] = [];
  readyState = 0;
  onerror: ((e: Event) => void) | null = null;
  listeners: Record<string, ((e: MessageEvent<string>) => void)[]> = {};
  constructor(readonly url: string) {
    FakeES.instances.push(this);
  }
  addEventListener(type: string, fn: (e: MessageEvent<string>) => void) {
    this.listeners[type] = [...(this.listeners[type] ?? []), fn];
  }
  close() {
    this.readyState = 2;
  }
  emit(snap: Snapshot) {
    this.readyState = 1;
    for (const fn of this.listeners.snapshot ?? [])
      fn({ data: JSON.stringify(snap) } as MessageEvent<string>);
  }
}

const preview = (name: string): Preview => ({
  name,
  title: name,
  project: "",
  created: "2026-01-01T00:00:00Z",
  expires: "2026-01-01T06:00:00Z",
  pinned: false,
  locked: false,
  url: `https://glim.example.com/${name}/`,
  views: 0,
  lastSeen: null,
});

const snap = (previews: Preview[]): Snapshot => ({
  now: "2026-01-01T00:00:00Z",
  previews,
  status: { live: previews.length, pinned: 0, diskBytes: 1, nextExpiry: null },
});

function setup(over: { closeMs?: number } = {}) {
  FakeES.instances = [];
  const clock = { t: 1_000_000 };
  const live = createLive({
    api: { previews: async () => snap([]), session: (async () => ({})) as never },
    onUnauthorized: () => {},
    EventSourceImpl: FakeES as unknown as typeof EventSource,
    retryMs: 1,
    now: () => clock.t,
    ...over,
  });
  return { live, clock };
}

test("a brief tab switch keeps the stream; a long one replaces it", () => {
  createRoot((dispose) => {
    const { live, clock } = setup();
    live.start();
    const first = FakeES.instances[0];
    live.hide();
    clock.t += 5_000;
    live.show();
    expect(first?.readyState).not.toBe(FakeES.CLOSED);
    expect(FakeES.instances.length).toBe(1);

    live.hide();
    clock.t += 31_000;
    live.show();
    expect(first?.readyState).toBe(FakeES.CLOSED);
    expect(FakeES.instances.length).toBe(2);
    live.stop();
    dispose();
  });
});

test("show without a prior hide never restarts the stream", () => {
  createRoot((dispose) => {
    const { live, clock } = setup();
    live.start();
    clock.t += 3_600_000;
    live.show();
    expect(FakeES.instances.length).toBe(1);
    live.stop();
    dispose();
  });
});

test("a card the server removes stays, closing, until its animation ends", async () => {
  await createRoot(async (dispose) => {
    const { live } = setup({ closeMs: 20 });
    live.start();
    const es = FakeES.instances[0] as FakeES;
    es.emit(snap([preview("a"), preview("b"), preview("c")]));
    flush();
    es.emit(snap([preview("a"), preview("c")]));
    flush();
    expect(live.state.previews.map((p) => p.name)).toEqual(["a", "b", "c"]);
    expect(live.leaving.b).toBe(true);
    expect(live.leaving.a).toBeFalsy();

    await new Promise((r) => setTimeout(r, 40));
    flush();
    expect(live.state.previews.map((p) => p.name)).toEqual(["a", "c"]);
    expect(live.leaving.b).toBe(false);
    live.stop();
    dispose();
  });
});

test("the first snapshot never ghosts anything", () => {
  createRoot((dispose) => {
    const { live } = setup();
    live.start();
    FakeES.instances[0]?.emit(snap([preview("a")]));
    flush();
    expect(live.state.previews.length).toBe(1);
    expect(live.leaving.a).toBeFalsy();
    live.stop();
    dispose();
  });
});

test("a preview that returns while closing is no longer leaving", () => {
  createRoot((dispose) => {
    const { live } = setup({ closeMs: 10_000 });
    live.start();
    const es = FakeES.instances[0] as FakeES;
    es.emit(snap([preview("a")]));
    flush();
    es.emit(snap([]));
    flush();
    expect(live.leaving.a).toBe(true);
    es.emit(snap([preview("a")]));
    flush();
    expect(live.leaving.a).toBe(false);
    expect(live.state.previews.length).toBe(1);
    live.stop();
    dispose();
  });
});

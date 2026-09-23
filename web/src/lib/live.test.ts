import { expect, test } from "bun:test";
import { createEffect, createRoot, flush } from "solid-js";
import { ApiError } from "./api";
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
  fail(closed: boolean) {
    this.readyState = closed ? 2 : 0;
    this.onerror?.(new Event("error"));
  }
}

const preview = (name: string, over: Partial<Preview> = {}): Preview => ({
  name,
  title: name,
  project: "",
  created: "2026-01-01T00:00:00Z",
  expires: "2026-01-01T06:00:00Z",
  pinned: false,
  url: `https://glim.example.com/${name}/`,
  ...over,
});

const snap = (previews: Preview[], now = "2026-01-01T00:00:00Z"): Snapshot => ({
  now,
  previews,
  status: { live: previews.length, pinned: 0, diskBytes: 1, nextExpiry: null },
});

const tick = () => new Promise((r) => setTimeout(r, 5));

function setup(session: () => Promise<unknown> = async () => ({})) {
  FakeES.instances = [];
  let signedOut = 0;
  let previewsCalls = 0;
  const live = createLive({
    api: {
      previews: async () => {
        previewsCalls++;
        return snap([preview("fresh")]);
      },
      session: session as never,
    },
    onUnauthorized: () => signedOut++,
    EventSourceImpl: FakeES as unknown as typeof EventSource,
    retryMs: 1,
    now: () => Date.parse("2026-01-01T00:00:00Z"),
  });
  return { live, signedOut: () => signedOut, previewsCalls: () => previewsCalls };
}

test("snapshots reconcile by name and only touch changed previews", () => {
  createRoot((dispose) => {
    const { live } = setup();
    live.start();
    const es = FakeES.instances[0]!;
    es.emit(snap([preview("a"), preview("b")]));
    flush();
    expect(live.conn()).toBe("live");
    let runsA = 0;
    let runsB = 0;
    createEffect(
      () => live.state.previews[0]?.expires,
      () => {
        runsA++;
      },
    );
    createEffect(
      () => live.state.previews[1]?.expires,
      () => {
        runsB++;
      },
    );
    flush();
    es.emit(snap([preview("a", { expires: "2026-01-02T00:00:00Z" }), preview("b")]));
    flush();
    expect(runsA).toBe(2);
    expect(runsB).toBe(1);
    // Republish in place: same name, new created → state reflects it.
    es.emit(snap([preview("a", { created: "2026-01-01T03:00:00Z" }), preview("b")]));
    flush();
    expect(live.state.previews[0]?.created).toBe("2026-01-01T03:00:00Z");
    es.emit(snap([preview("b")]));
    flush();
    expect(live.state.previews.map((p) => p.name)).toEqual(["b"]);
    live.stop();
    dispose();
  });
});

test("skew follows server time", () => {
  createRoot((dispose) => {
    const { live } = setup();
    live.start();
    FakeES.instances[0]!.emit(snap([], "2026-01-01T00:00:30Z"));
    flush();
    expect(live.skew()).toBe(30_000);
    live.stop();
    dispose();
  });
});

test("closed stream + 401 → onUnauthorized", async () => {
  await createRoot(async (dispose) => {
    const { live, signedOut } = setup(async () => {
      throw new ApiError(401, "unauthorized", "Sign in to continue.");
    });
    live.start();
    FakeES.instances[0]!.fail(true);
    flush();
    expect(live.conn()).toBe("reconnecting");
    await tick();
    expect(signedOut()).toBe(1);
    expect(FakeES.instances.length).toBe(1);
    live.stop();
    dispose();
  });
});

test("closed stream + valid session → reopens", async () => {
  await createRoot(async (dispose) => {
    const { live, signedOut } = setup();
    live.start();
    FakeES.instances[0]!.fail(true);
    await tick();
    expect(signedOut()).toBe(0);
    expect(FakeES.instances.length).toBe(2);
    live.stop();
    dispose();
  });
});

test("refresh (tab visible again) refetches and applies", async () => {
  await createRoot(async (dispose) => {
    const { live, previewsCalls } = setup();
    await live.refresh();
    flush();
    expect(previewsCalls()).toBe(1);
    expect(live.state.previews[0]?.name).toBe("fresh");
    expect(live.state.loaded).toBe(true);
    dispose();
  });
});

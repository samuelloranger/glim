import { expect, test } from "bun:test";
import { createMemo, createRoot, flush } from "solid-js";
import { createActions } from "./actions";
import { ApiError } from "./api";
import type { Preview } from "./types";

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const base: Preview = {
  name: "diff-a1b2",
  title: "Diff",
  project: "",
  created: "2026-01-01T00:00:00Z",
  expires: "2026-01-01T06:00:00Z",
  pinned: false,
  url: "https://glim.example.com/diff-a1b2/",
};

function harness() {
  const calls = { patch: [] as Preview[], drop: [] as string[], toasts: [] as [string, string][] };
  const pending = {
    pin: deferred<Preview>(),
    extend: deferred<Preview>(),
    remove: deferred<void>(),
  };
  const actions = createActions({
    api: {
      pin: () => pending.pin.promise,
      extend: () => pending.extend.promise,
      remove: () => pending.remove.promise,
    },
    live: { patch: (p) => calls.patch.push(p), drop: (n) => calls.drop.push(n) },
    toast: (text, tone = "info") => calls.toasts.push([text, tone]),
    now: () => Date.parse("2026-01-01T01:00:00Z"),
    closeMs: 1,
  });
  return { actions, calls, pending };
}

test("pin shows immediately, then patches and confirms", async () => {
  await createRoot(async (dispose) => {
    const { actions, calls, pending } = harness();
    const pinned = createMemo(() => actions.overrides[base.name]?.pinned === true);
    flush();
    pinned();
    const done = actions.pin(base.name);
    flush();
    expect(pinned()).toBe(true);
    pending.pin.resolve({ ...base, pinned: true });
    await done;
    expect(calls.patch[0]?.pinned).toBe(true);
    expect(calls.toasts).toEqual([["Pinned", "info"]]);
    dispose();
  });
});

test("failed extend reverts and shows the server message", async () => {
  await createRoot(async (dispose) => {
    const { actions, calls, pending } = harness();
    const expires = createMemo(() => actions.overrides[base.name]?.expires);
    flush();
    expires();
    const done = actions.extend(base.name, "6h", "6h");
    flush();
    expect(expires()).toBe("2026-01-01T07:00:00.000Z");
    pending.extend.reject(new ApiError(404, "not_found", "That preview no longer exists."));
    await done;
    await new Promise((r) => setTimeout(r, 5));
    flush();
    expect(expires()).toBeUndefined();
    expect(calls.patch.length).toBe(0);
    expect(calls.toasts).toEqual([["That preview no longer exists.", "error"]]);
    dispose();
  });
});

test("remove marks the card closing, then drops it", async () => {
  await createRoot(async (dispose) => {
    const { actions, calls, pending } = harness();
    const removed = createMemo(() => actions.overrides[base.name]?.removed === true);
    flush();
    removed();
    const done = actions.remove(base.name);
    flush();
    expect(removed()).toBe(true);
    pending.remove.resolve();
    await done;
    expect(calls.drop).toEqual([base.name]);
    expect(calls.toasts).toEqual([[`Removed ${base.name}`, "info"]]);
    dispose();
  });
});

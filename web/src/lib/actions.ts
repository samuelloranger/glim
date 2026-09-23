import { action, createOptimisticStore } from "solid-js";
import { type Api, errorText } from "./api";
import type { Live } from "./live";
import { durationMs } from "./time";
import type { Preview } from "./types";

export type Override = { pinned?: boolean; expires?: string; removed?: boolean };

export const CLOSE_MS = 220;

const wait = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

type ActionDeps = {
  api: Pick<Api, "extend" | "pin" | "remove">;
  live: Pick<Live, "patch" | "drop">;
  toast: (text: string, tone?: "info" | "error") => void;
  now: () => number;
  closeMs?: number;
};

/**
 * Optimistic overrides layered over the live store. The base list is never
 * rebuilt, so unrelated cards (and their iframes) are untouched; overrides
 * revert on their own when each action settles.
 */
export function createActions(deps: ActionDeps) {
  const closeMs = deps.closeMs ?? CLOSE_MS;
  const [overrides, setOverrides] = createOptimisticStore<Record<string, Override>>(() => ({}), {});

  const extendAction = action(function* (name: string, ttl: string) {
    const expires = new Date(deps.now() + durationMs(ttl)).toISOString();
    setOverrides((d) => {
      d[name] = { ...d[name], expires };
    });
    const p = (yield deps.api.extend(name, ttl)) as Preview;
    deps.live.patch(p);
  });

  const pinAction = action(function* (name: string) {
    setOverrides((d) => {
      d[name] = { ...d[name], pinned: true };
    });
    const p = (yield deps.api.pin(name)) as Preview;
    deps.live.patch(p);
  });

  const removeAction = action(function* (name: string) {
    setOverrides((d) => {
      d[name] = { ...d[name], removed: true };
    });
    yield Promise.all([deps.api.remove(name), wait(closeMs)]);
    deps.live.drop(name);
  });

  async function run(p: Promise<unknown>, success: string) {
    try {
      await p;
      deps.toast(success);
    } catch (e) {
      deps.toast(errorText(e), "error");
    }
  }

  return {
    overrides,
    extend: (name: string, ttl: string, label: string) =>
      run(extendAction(name, ttl), `Extended to ${label}`),
    pin: (name: string) => run(pinAction(name), "Pinned"),
    remove: (name: string) => run(removeAction(name), `Removed ${name}`),
  };
}

export type Actions = ReturnType<typeof createActions>;

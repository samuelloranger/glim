import { createSignal } from "solid-js";

/** One shared 1 s tick, corrected by the server clock skew. */
export function createClock(skew: () => number, intervalMs = 1000) {
  const [tick, setTick] = createSignal(Date.now());
  const now = () => tick() + skew();
  function start() {
    const id = setInterval(() => setTick(Date.now()), intervalMs);
    return () => clearInterval(id);
  }
  return { now, start };
}

export const EMBER_MS = 15 * 60_000;
const MAX_TTL_MS = 8760 * 3_600_000;
const UNIT_MS = { d: 86_400_000, h: 3_600_000, m: 60_000, s: 1000 } as const;

export function formatLeft(ms: number): string {
  if (ms <= 0) return "expired";
  const s = Math.floor(ms / 1000);
  const d = Math.floor(s / 86_400);
  const h = Math.floor((s % 86_400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${sec}s`;
  return `${sec}s`;
}

/** Remaining share of a preview's lifetime, 1 = just published, 0 = expired. */
export function lifeFraction(created: string, expires: string, nowMs: number): number {
  const c = Date.parse(created);
  const e = Date.parse(expires);
  if (!(e > c)) return 0;
  return Math.min(1, Math.max(0, (e - nowMs) / (e - c)));
}

export function skewMs(serverNow: string, clientNowMs: number): number {
  return Date.parse(serverNow) - clientNowMs;
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  const units = ["KiB", "MiB", "GiB", "TiB"];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(1)} ${units[i]}`;
}

/** Friendly lifetime ("90m", "1h 30m", "7d") → Go duration in seconds, or null. */
export function parseTTL(input: string): string | null {
  const s = input.trim().toLowerCase().replace(/\s+/g, "");
  if (!/^(\d+[dhms])+$/.test(s)) return null;
  let ms = 0;
  for (const [, n, u] of s.matchAll(/(\d+)([dhms])/g)) {
    ms += Number(n) * UNIT_MS[u as keyof typeof UNIT_MS];
  }
  if (ms < 1000 || ms > MAX_TTL_MS) return null;
  return `${Math.floor(ms / 1000)}s`;
}

/** Milliseconds in a Go duration made of h/m/s parts ("6h", "1h30m", "5400s"). */
export function durationMs(goDuration: string): number {
  let ms = 0;
  for (const [, n, u] of goDuration.matchAll(/(\d+)([hms])/g)) {
    ms += Number(n) * UNIT_MS[u as "h" | "m" | "s"];
  }
  return ms;
}

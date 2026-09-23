import { describe, expect, test } from "bun:test";
import { durationMs, formatBytes, formatLeft, lifeFraction, parseTTL, skewMs } from "./time";

describe("formatLeft", () => {
  test.each([
    [0, "expired"],
    [-5, "expired"],
    [42_000, "42s"],
    [11 * 60_000 + 40_000, "11m 40s"],
    [4 * 3_600_000 + 12 * 60_000 + 59_000, "4h 12m"],
    [2 * 86_400_000 + 3 * 3_600_000, "2d 3h"],
  ])("%p ms → %p", (ms, want) => expect(formatLeft(ms)).toBe(want));
});

test("lifeFraction drains from 1 to 0 and clamps", () => {
  const c = "2026-01-01T00:00:00Z";
  const e = "2026-01-01T10:00:00Z";
  const at = (iso: string) => Date.parse(iso);
  expect(lifeFraction(c, e, at(c))).toBe(1);
  expect(lifeFraction(c, e, at("2026-01-01T05:00:00Z"))).toBeCloseTo(0.5);
  expect(lifeFraction(c, e, at("2026-01-02T00:00:00Z"))).toBe(0);
  expect(lifeFraction(c, e, at("2025-12-31T00:00:00Z"))).toBe(1);
  expect(lifeFraction(e, c, at(c))).toBe(0);
});

test("skewMs is server minus client", () => {
  expect(skewMs("2026-01-01T00:00:10Z", Date.parse("2026-01-01T00:00:00Z"))).toBe(10_000);
});

test("formatBytes", () => {
  expect(formatBytes(0)).toBe("0 B");
  expect(formatBytes(512)).toBe("512 B");
  expect(formatBytes(1536)).toBe("1.5 KiB");
  expect(formatBytes(48 * 1024 * 1024)).toBe("48.0 MiB");
});

test("parseTTL accepts friendly input and emits Go durations", () => {
  expect(parseTTL("90m")).toBe("5400s");
  expect(parseTTL(" 1h 30m ")).toBe("5400s");
  expect(parseTTL("7d")).toBe("604800s");
  expect(parseTTL("2D12H")).toBe("216000s");
  for (const bad of ["", "abc", "0m", "10", "1y", "366d", "-1h"]) expect(parseTTL(bad)).toBeNull();
});

test("durationMs parses Go durations used by presets and parseTTL", () => {
  expect(durationMs("6h")).toBe(6 * 3_600_000);
  expect(durationMs("168h")).toBe(7 * 86_400_000);
  expect(durationMs("5400s")).toBe(5_400_000);
  expect(durationMs("1h30m")).toBe(5_400_000);
});

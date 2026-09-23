import { expect, test } from "bun:test";
import { displayTitle, thumbnailSrc } from "./preview";

test("thumbnailSrc changes when a preview is republished in place", () => {
  const a = thumbnailSrc({ name: "report-k3n7", created: "2026-01-01T00:00:00Z" });
  const b = thumbnailSrc({ name: "report-k3n7", created: "2026-01-01T01:00:00Z" });
  expect(a.startsWith("/report-k3n7/?v=")).toBe(true);
  expect(a).not.toBe(b);
});

test("displayTitle falls back to the slug", () => {
  expect(displayTitle({ title: "Diff review", name: "diff-review-a1b2" })).toBe("Diff review");
  expect(displayTitle({ title: "  ", name: "diff-review-a1b2" })).toBe("diff-review-a1b2");
});

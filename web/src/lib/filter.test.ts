import { expect, test } from "bun:test";
import { filtersFromSearch, filtersToSearch, matches, projectsOf } from "./filter";
import type { Preview } from "./types";

const p = (over: Partial<Preview>): Preview => ({
  name: "diff-review-a1b2",
  title: "Diff review",
  project: "api",
  created: "2026-01-01T00:00:00Z",
  expires: "2026-01-01T06:00:00Z",
  pinned: false,
  url: "https://glim.example.com/diff-review-a1b2/",
  ...over,
});

test("matches searches title, slug and project, case-insensitively", () => {
  expect(matches(p({}), { q: "", project: "" })).toBe(true);
  expect(matches(p({}), { q: "DIFF", project: "" })).toBe(true);
  expect(matches(p({}), { q: "a1b2", project: "" })).toBe(true);
  expect(matches(p({}), { q: "api", project: "" })).toBe(true);
  expect(matches(p({}), { q: "login", project: "" })).toBe(false);
  expect(matches(p({}), { q: "", project: "web" })).toBe(false);
  expect(matches(p({}), { q: "diff", project: "api" })).toBe(true);
});

test("projectsOf is sorted, unique, non-empty", () => {
  expect(
    projectsOf([
      p({ project: "web" }),
      p({ project: "" }),
      p({ project: "api" }),
      p({ project: "web" }),
    ]),
  ).toEqual(["api", "web"]);
});

test("filters round-trip through the query string", () => {
  expect(filtersFromSearch("?q=diff&project=api")).toEqual({ q: "diff", project: "api" });
  expect(filtersFromSearch("")).toEqual({ q: "", project: "" });
  expect(filtersToSearch({ q: "", project: "" })).toBe("");
  expect(filtersToSearch({ q: "a b", project: "api" })).toBe("?q=a+b&project=api");
});

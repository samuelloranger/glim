import type { Preview } from "./types";

export type Filters = { q: string; project: string };

export function matches(p: Preview, f: Filters): boolean {
  if (f.project && p.project !== f.project) return false;
  const needle = f.q.trim().toLowerCase();
  if (!needle) return true;
  return [p.title, p.name, p.project].some((field) => field.toLowerCase().includes(needle));
}

export function projectsOf(list: readonly Preview[]): string[] {
  return [...new Set(list.map((p) => p.project).filter(Boolean))].sort();
}

export function filtersFromSearch(search: string): Filters {
  const params = new URLSearchParams(search);
  return { q: params.get("q") ?? "", project: params.get("project") ?? "" };
}

export function filtersToSearch(f: Filters): string {
  const params = new URLSearchParams();
  if (f.q) params.set("q", f.q);
  if (f.project) params.set("project", f.project);
  const s = params.toString();
  return s ? `?${s}` : "";
}

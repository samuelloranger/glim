import { For } from "solid-js";
import type { Filters as FilterValue } from "../lib/filter";

export function Filters(props: {
  value: FilterValue;
  projects: string[];
  onChange: (f: FilterValue) => void;
}) {
  return (
    <search class="filters">
      <input
        class="input search"
        type="search"
        placeholder="Search previews"
        aria-label="Search previews"
        value={props.value.q}
        onInput={(e) => props.onChange({ ...props.value, q: e.currentTarget.value })}
      />
      <select
        class="input"
        aria-label="Project"
        value={props.value.project}
        onChange={(e) => props.onChange({ ...props.value, project: e.currentTarget.value })}
      >
        <option value="">All projects</option>
        <For each={props.projects}>{(p) => <option value={p}>{p}</option>}</For>
      </select>
    </search>
  );
}

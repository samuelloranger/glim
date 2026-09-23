import { For } from "solid-js";
import type { Toasts } from "../lib/toasts";

export function ToastList(props: { toasts: Toasts }) {
  return (
    <div class="toasts" role="status" aria-live="polite">
      <For each={props.toasts.toasts.list}>
        {(t) => (
          <div class={["toast", { error: t.tone === "error" }]}>
            <span>{t.text}</span>
            <button
              class="btn quiet"
              type="button"
              aria-label="Dismiss"
              onClick={() => props.toasts.dismiss(t.id)}
            >
              ×
            </button>
          </div>
        )}
      </For>
    </div>
  );
}

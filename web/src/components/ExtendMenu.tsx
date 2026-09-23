import { createSignal, For, onSettled, Show } from "solid-js";
import { parseTTL } from "../lib/time";

const PRESETS: readonly (readonly [string, string])[] = [
  ["1h", "1h"],
  ["6h", "6h"],
  ["24h", "24h"],
  ["168h", "7d"],
];

export function ExtendMenu(props: { onPick: (ttl: string, label: string) => void }) {
  const [open, setOpen] = createSignal(false);
  const [custom, setCustom] = createSignal("");
  const [error, setError] = createSignal("");
  let wrap: HTMLDivElement | undefined;
  let button: HTMLButtonElement | undefined;

  function close() {
    setOpen(false);
    setCustom("");
    setError("");
  }

  function pick(ttl: string, label: string) {
    props.onPick(ttl, label);
    close();
    button?.focus();
  }

  function submitCustom(e: SubmitEvent) {
    e.preventDefault();
    const ttl = parseTTL(custom());
    if (!ttl) {
      setError("Use a lifetime like 90m, 12h or 3d, up to 365d.");
      return;
    }
    pick(ttl, custom().trim());
  }

  onSettled(() => {
    const outside = (e: PointerEvent) => {
      if (open() && wrap && !wrap.contains(e.target as Node)) close();
    };
    document.addEventListener("pointerdown", outside);
    return () => document.removeEventListener("pointerdown", outside);
  });

  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: Escape handles the open menu from any child.
    <div
      class="extend"
      ref={(el) => (wrap = el)}
      onKeyDown={(e) => {
        if (e.key === "Escape" && open()) {
          close();
          button?.focus();
        }
      }}
    >
      <button
        ref={(el) => (button = el)}
        class="btn quiet"
        type="button"
        aria-expanded={open() ? "true" : "false"}
        onClick={() => (open() ? close() : setOpen(true))}
      >
        Extend
      </button>
      <Show when={open()}>
        {/* biome-ignore lint/a11y/useSemanticElements: The menu groups buttons and a separate form. */}
        <div class="menu" role="group" aria-label="Extend to">
          <p class="menu-title">Extend to</p>
          <div class="menu-presets">
            <For each={PRESETS}>
              {(preset) => (
                <button class="btn" type="button" onClick={() => pick(preset[0], preset[1])}>
                  {preset[1]}
                </button>
              )}
            </For>
          </div>
          <form class="menu-custom" onSubmit={submitCustom}>
            <input
              class="input"
              aria-label="Custom lifetime"
              placeholder="90m, 12h, 3d"
              value={custom()}
              onInput={(e) => setCustom(e.currentTarget.value)}
            />
            <button class="btn" type="submit">
              Set
            </button>
          </form>
          <Show when={error()}>
            <p class="error" role="alert">
              {error()}
            </p>
          </Show>
        </div>
      </Show>
    </div>
  );
}

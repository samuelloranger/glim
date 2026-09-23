import { createMemo, onSettled, Show, untrack } from "solid-js";
import type { Override } from "../lib/actions";
import { displayTitle, thumbnailSrc } from "../lib/preview";
import { EMBER_MS, formatLeft, lifeFraction } from "../lib/time";
import type { Preview } from "../lib/types";
import { ExtendMenu } from "./ExtendMenu";
import { Lifeline } from "./Lifeline";

const THUMB_WIDTH = 1280;

export function PreviewCard(props: {
  preview: Preview;
  override?: Override;
  now: number;
  arriving: boolean;
  onExtend: (ttl: string, label: string) => void;
  onPin: () => void;
  onRemove: () => void;
  onCopy: () => void;
}) {
  // Decided once at mount: only previews that appear after the first load open.
  const arriving = untrack(() => props.arriving);
  const title = createMemo(() => displayTitle(props.preview));
  const pinned = createMemo(() => props.override?.pinned ?? props.preview.pinned);
  const expires = createMemo(() => props.override?.expires ?? props.preview.expires);
  const leftMs = createMemo(() => Date.parse(expires()) - props.now);
  const ember = createMemo(() => !pinned() && leftMs() < EMBER_MS);
  const closing = createMemo(
    () => props.override?.removed === true || (!pinned() && leftMs() <= 0),
  );
  const href = () => `/${props.preview.name}/`;
  let windowEl: HTMLAnchorElement | undefined;

  onSettled(() => {
    const el = windowEl;
    if (!el) return;
    const ro = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width;
      if (w) el.style.setProperty("--scale", String(w / THUMB_WIDTH));
    });
    ro.observe(el);
    return () => ro.disconnect();
  });

  return (
    <article class={["card", { arriving, closing: closing() }]} aria-label={title()}>
      <a
        ref={(el) => (windowEl = el)}
        class="window"
        href={href()}
        target="_blank"
        rel="noopener"
        aria-label={`Open ${title()}`}
      >
        <span class="visually-hidden">Open {title()}</span>
        <iframe
          src={thumbnailSrc(props.preview)}
          sandbox="allow-scripts"
          loading="lazy"
          tabindex="-1"
          aria-hidden="true"
          title={`Preview of ${title()}`}
        />
      </a>
      <Lifeline
        fraction={lifeFraction(props.preview.created, expires(), props.now)}
        pinned={pinned()}
        ember={ember()}
      />
      <div class="meta">
        <h2 class="title">{title()}</h2>
        <span class={["left", { ember: ember() }]}>
          {pinned() ? "pinned" : formatLeft(leftMs())}
        </span>
        <code class="slug">{props.preview.name}</code>
        <Show when={props.preview.project}>
          <span class="chip">{props.preview.project}</span>
        </Show>
      </div>
      <div class="actions">
        <a class="btn quiet" href={href()} target="_blank" rel="noopener">
          Open
        </a>
        <button class="btn quiet" type="button" onClick={() => props.onCopy()}>
          Copy link
        </button>
        <ExtendMenu onPick={(ttl, label) => props.onExtend(ttl, label)} />
        <Show when={!pinned()}>
          <button class="btn quiet" type="button" onClick={() => props.onPin()}>
            Pin
          </button>
        </Show>
        <button class="btn quiet danger" type="button" onClick={() => props.onRemove()}>
          Remove
        </button>
      </div>
    </article>
  );
}

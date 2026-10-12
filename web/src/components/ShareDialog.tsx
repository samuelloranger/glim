import { createEffect, createSignal, Show } from "solid-js";
import { qrSrc } from "../lib/preview";
import type { Preview } from "../lib/types";

/**
 * The preview's link as a QR code to scan from another device, with copy and,
 * where the browser has one, the system share sheet.
 */
export function ShareDialog(props: {
  preview: Preview | null;
  title: string;
  onClose: () => void;
}) {
  let dialog: HTMLDialogElement | undefined;
  let urlInput: HTMLInputElement | undefined;
  // Feedback stays inside the dialog: toasts sit under the modal backdrop.
  const [copied, setCopied] = createSignal<"" | "ok" | "failed">("");
  const canShare = typeof navigator !== "undefined" && typeof navigator.share === "function";

  createEffect(
    () => props.preview !== null,
    (open) => {
      if (!dialog) return;
      if (open && !dialog.open) {
        setCopied("");
        dialog.showModal();
      }
      if (!open && dialog.open) dialog.close();
    },
  );

  async function copy(p: Preview) {
    try {
      await navigator.clipboard.writeText(p.url);
      setCopied("ok");
    } catch {
      setCopied("failed");
      urlInput?.select();
    }
  }

  async function share(p: Preview) {
    try {
      await navigator.share({ title: props.title, url: p.url });
      props.onClose();
    } catch {
      // Cancelled, or refused by the browser: the dialog stays open.
    }
  }

  return (
    <dialog
      ref={(el) => (dialog = el)}
      class="dialog share"
      aria-labelledby="share-title"
      onClose={() => {
        if (props.preview) props.onClose();
      }}
    >
      <h2 id="share-title">Share {props.title}</h2>
      <Show when={props.preview}>
        {(p) => (
          <>
            <img class="qr" src={qrSrc(p())} alt="QR code of the link" width="200" height="200" />
            <input
              ref={(el) => (urlInput = el)}
              class="input share-url"
              readonly
              aria-label="Link"
              value={p().url}
              onFocus={(e) => e.currentTarget.select()}
            />
            <Show when={copied() === "failed"}>
              <p class="help" role="alert">
                Couldn't copy the link. It's selected above: copy it from there.
              </p>
            </Show>
            <div class="dialog-actions">
              <button class="btn" type="button" onClick={() => props.onClose()}>
                Done
              </button>
              <Show when={canShare}>
                <button class="btn" type="button" onClick={() => void share(p())}>
                  Share…
                </button>
              </Show>
              <button class="btn primary" type="button" onClick={() => void copy(p())}>
                {copied() === "ok" ? "Copied" : "Copy link"}
              </button>
            </div>
          </>
        )}
      </Show>
    </dialog>
  );
}

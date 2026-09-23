import { createEffect } from "solid-js";

/** Native modal dialog: traps focus, Escape cancels, focus returns on close. */
export function ConfirmDialog(props: {
  open: boolean;
  title: string;
  body: string;
  confirmLabel: string;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  let dialog: HTMLDialogElement | undefined;
  createEffect(
    () => props.open,
    (open) => {
      if (!dialog) return;
      if (open && !dialog.open) dialog.showModal();
      if (!open && dialog.open) dialog.close();
    },
  );
  return (
    <dialog
      ref={(el) => (dialog = el)}
      class="dialog"
      aria-labelledby="confirm-title"
      onClose={() => {
        if (props.open) props.onCancel();
      }}
    >
      <h2 id="confirm-title">{props.title}</h2>
      <p class="help">{props.body}</p>
      <div class="dialog-actions">
        <button class="btn" type="button" onClick={() => props.onCancel()}>
          Cancel
        </button>
        <button class="btn primary" type="button" onClick={() => props.onConfirm()}>
          {props.confirmLabel}
        </button>
      </div>
    </dialog>
  );
}

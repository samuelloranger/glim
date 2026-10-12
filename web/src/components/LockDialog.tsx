import { createEffect, createSignal, Show } from "solid-js";
import { errorText } from "../lib/api";

/**
 * Asks for a new preview password. onLock resolves when the server accepted
 * it; a rejection (too short, preview gone) is shown inside the dialog.
 */
export function LockDialog(props: {
  open: boolean;
  title: string;
  onLock: (password: string) => Promise<void>;
  onCancel: () => void;
}) {
  let dialog: HTMLDialogElement | undefined;
  const [password, setPassword] = createSignal("");
  const [repeat, setRepeat] = createSignal("");
  const [error, setError] = createSignal("");
  const [busy, setBusy] = createSignal(false);

  createEffect(
    () => props.open,
    (open) => {
      if (!dialog) return;
      if (open && !dialog.open) {
        setPassword("");
        setRepeat("");
        setError("");
        dialog.showModal();
      }
      if (!open && dialog.open) dialog.close();
    },
  );

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (password() !== repeat()) {
      setError("The passwords don't match.");
      return;
    }
    setError("");
    setBusy(true);
    try {
      await props.onLock(password());
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <dialog
      ref={(el) => (dialog = el)}
      class="dialog"
      aria-labelledby="lock-title"
      onClose={() => {
        if (props.open) props.onCancel();
      }}
    >
      <h2 id="lock-title">Lock {props.title}</h2>
      <p class="help">
        Visitors need this password before they see anything. You stay signed in, so you won't be
        asked for it.
      </p>
      <form class="auth-form" onSubmit={submit}>
        <div class="field">
          <label for="lock-password">Password</label>
          <input
            id="lock-password"
            class="input"
            type="password"
            autocomplete="new-password"
            minlength="8"
            required
            value={password()}
            onInput={(e) => setPassword(e.currentTarget.value)}
          />
        </div>
        <div class="field">
          <label for="lock-repeat">Repeat password</label>
          <input
            id="lock-repeat"
            class="input"
            type="password"
            autocomplete="new-password"
            required
            value={repeat()}
            onInput={(e) => setRepeat(e.currentTarget.value)}
          />
        </div>
        <Show when={error()}>
          <p class="error" role="alert">
            {error()}
          </p>
        </Show>
        <div class="dialog-actions">
          <button class="btn" type="button" onClick={() => props.onCancel()}>
            Cancel
          </button>
          <button class="btn primary" type="submit" disabled={busy()}>
            Lock
          </button>
        </div>
      </form>
    </dialog>
  );
}

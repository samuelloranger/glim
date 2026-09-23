import { createEffect, createSignal, For, Show } from "solid-js";
import { api, errorText } from "../lib/api";
import type { User } from "../lib/types";

export function AccountPanel(props: {
  open: boolean;
  user: User;
  onClose: () => void;
  onSignOut: () => void;
  toast: (text: string, tone?: "info" | "error") => void;
}) {
  let dialog: HTMLDialogElement | undefined;
  const [users, setUsers] = createSignal<User[]>([]);
  const [removing, setRemoving] = createSignal<string | null>(null);
  const [current, setCurrent] = createSignal("");
  const [next, setNext] = createSignal("");
  const [confirmNext, setConfirmNext] = createSignal("");
  const [pwError, setPwError] = createSignal("");
  const [newName, setNewName] = createSignal("");
  const [newPass, setNewPass] = createSignal("");
  const [addError, setAddError] = createSignal("");

  async function loadUsers() {
    try {
      setUsers((await api.users()).users);
    } catch (e) {
      props.toast(errorText(e), "error");
    }
  }

  createEffect(
    () => props.open,
    (open) => {
      if (!dialog) return;
      if (open && !dialog.open) {
        dialog.showModal();
        void loadUsers();
      }
      if (!open && dialog.open) dialog.close();
    },
  );

  async function changePassword(e: SubmitEvent) {
    e.preventDefault();
    if (next() !== confirmNext()) {
      setPwError("New passwords don't match.");
      return;
    }
    setPwError("");
    try {
      await api.changePassword(current(), next());
      setCurrent("");
      setNext("");
      setConfirmNext("");
      props.toast("Password changed. Other devices were signed out.");
    } catch (err) {
      setPwError(errorText(err));
    }
  }

  async function addUser(e: SubmitEvent) {
    e.preventDefault();
    setAddError("");
    try {
      const u = await api.addUser(newName(), newPass());
      setNewName("");
      setNewPass("");
      props.toast(`Added ${u.email}`);
      await loadUsers();
    } catch (err) {
      setAddError(errorText(err));
    }
  }

  async function removeUser(email: string) {
    setRemoving(null);
    try {
      await api.removeUser(email);
      props.toast(`Removed ${email}`);
      await loadUsers();
    } catch (err) {
      props.toast(errorText(err), "error");
    }
  }

  return (
    <dialog
      ref={(el) => (dialog = el)}
      class="panel"
      aria-labelledby="account-title"
      onClose={() => {
        if (props.open) props.onClose();
      }}
    >
      <div class="panel-head">
        <h2 id="account-title">Signed in as {props.user.email}</h2>
        <button class="btn quiet" type="button" onClick={() => props.onClose()}>
          Close
        </button>
      </div>
      <button class="btn" type="button" onClick={() => props.onSignOut()}>
        Sign out
      </button>

      <section class="panel-section" aria-labelledby="pw-title">
        <h3 id="pw-title">Change password</h3>
        <form class="auth-form" onSubmit={changePassword}>
          <div class="field">
            <label for="pw-current">Current password</label>
            <input
              id="pw-current"
              class="input"
              type="password"
              autocomplete="current-password"
              required
              value={current()}
              onInput={(e) => setCurrent(e.currentTarget.value)}
            />
          </div>
          <div class="field">
            <label for="pw-next">New password</label>
            <input
              id="pw-next"
              class="input"
              type="password"
              autocomplete="new-password"
              minlength="12"
              required
              value={next()}
              onInput={(e) => setNext(e.currentTarget.value)}
            />
          </div>
          <div class="field">
            <label for="pw-confirm">Confirm new password</label>
            <input
              id="pw-confirm"
              class="input"
              type="password"
              autocomplete="new-password"
              required
              value={confirmNext()}
              onInput={(e) => setConfirmNext(e.currentTarget.value)}
            />
          </div>
          <Show when={pwError()}>
            <p class="error" role="alert">
              {pwError()}
            </p>
          </Show>
          <button class="btn primary" type="submit">
            Change password
          </button>
        </form>
      </section>

      <section class="panel-section" aria-labelledby="users-title">
        <h3 id="users-title">People with access</h3>
        <ul class="users">
          <For each={users()}>
            {(u) => (
              <li>
                <span>{u.email}</span>
                <Show when={u.email !== props.user.email} fallback={<span class="help">You</span>}>
                  <Show
                    when={removing() === u.email}
                    fallback={
                      <button
                        class="btn quiet danger"
                        type="button"
                        onClick={() => setRemoving(u.email)}
                      >
                        Remove
                      </button>
                    }
                  >
                    <span class="confirm-inline">
                      <button
                        class="btn danger"
                        type="button"
                        onClick={() => void removeUser(u.email)}
                      >
                        Remove {u.email}
                      </button>
                      <button class="btn quiet" type="button" onClick={() => setRemoving(null)}>
                        Cancel
                      </button>
                    </span>
                  </Show>
                </Show>
              </li>
            )}
          </For>
        </ul>
        <form class="auth-form" onSubmit={addUser}>
          <div class="field">
            <label for="add-name">Email</label>
            <input
              id="add-name"
              class="input"
              type="email"
              autocomplete="off"
              autocapitalize="none"
              required
              value={newName()}
              onInput={(e) => setNewName(e.currentTarget.value)}
            />
          </div>
          <div class="field">
            <label for="add-pass">Password</label>
            <input
              id="add-pass"
              class="input"
              type="password"
              autocomplete="new-password"
              minlength="12"
              required
              value={newPass()}
              onInput={(e) => setNewPass(e.currentTarget.value)}
            />
          </div>
          <Show when={addError()}>
            <p class="error" role="alert">
              {addError()}
            </p>
          </Show>
          <button class="btn" type="submit">
            Add person
          </button>
        </form>
      </section>
    </dialog>
  );
}

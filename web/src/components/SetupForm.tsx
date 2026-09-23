import { createSignal, Show } from "solid-js";
import { api, errorText } from "../lib/api";
import type { SessionInfo } from "../lib/types";
import { AuthShell } from "./AuthShell";

export function SetupForm(props: { onDone: (info: SessionInfo) => void }) {
  const [email, setEmail] = createSignal("");
  const [password, setPassword] = createSignal("");
  const [confirm, setConfirm] = createSignal("");
  const [error, setError] = createSignal("");
  const [busy, setBusy] = createSignal(false);
  const [glance, setGlance] = createSignal(false);

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (password() !== confirm()) {
      setError("Passwords don't match.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      props.onDone(await api.setup(email(), password()));
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthShell title="Create your account" glance={glance()}>
      <form class="auth-form" onSubmit={submit}>
        <div class="field">
          <label for="setup-email">Email</label>
          <input
            id="setup-email"
            class="input"
            type="email"
            autocomplete="email"
            autocapitalize="none"
            spellcheck="false"
            required
            value={email()}
            onInput={(e) => setEmail(e.currentTarget.value)}
          />
        </div>
        <div class="field">
          <label for="setup-password">Password</label>
          <input
            id="setup-password"
            class="input"
            type="password"
            autocomplete="new-password"
            minlength="12"
            required
            value={password()}
            onInput={(e) => setPassword(e.currentTarget.value)}
            onFocus={() => setGlance(true)}
            onBlur={() => setGlance(false)}
            aria-describedby="setup-password-help"
          />
          <p id="setup-password-help" class="help">
            At least 12 characters.
          </p>
        </div>
        <div class="field">
          <label for="setup-confirm">Confirm password</label>
          <input
            id="setup-confirm"
            class="input"
            type="password"
            autocomplete="new-password"
            required
            value={confirm()}
            onInput={(e) => setConfirm(e.currentTarget.value)}
          />
        </div>
        <Show when={error()}>
          <p class="error" role="alert">
            {error()}
          </p>
        </Show>
        <button class="btn primary" type="submit" disabled={busy()}>
          Create account
        </button>
      </form>
    </AuthShell>
  );
}

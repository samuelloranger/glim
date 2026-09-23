import { createSignal, Show } from "solid-js";
import { api, errorText } from "../lib/api";
import type { SessionInfo } from "../lib/types";
import { AuthShell } from "./AuthShell";

export function SetupForm(props: { onDone: (info: SessionInfo) => void }) {
  const [code, setCode] = createSignal("");
  const [username, setUsername] = createSignal("");
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
      props.onDone(await api.setup(code(), username(), password()));
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
          <label for="setup-code">Setup code</label>
          <input
            id="setup-code"
            class="input mono"
            autocomplete="one-time-code"
            autocapitalize="characters"
            spellcheck="false"
            required
            value={code()}
            onInput={(e) => setCode(e.currentTarget.value)}
            aria-describedby="setup-code-help"
          />
          <p id="setup-code-help" class="help">
            Printed in the <code>glim serve</code> log, and by <code>glim status</code>.
          </p>
        </div>
        <div class="field">
          <label for="setup-username">Username</label>
          <input
            id="setup-username"
            class="input"
            autocomplete="username"
            autocapitalize="none"
            required
            value={username()}
            onInput={(e) => setUsername(e.currentTarget.value)}
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

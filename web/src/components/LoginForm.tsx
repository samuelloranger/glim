import { createSignal, Show } from "solid-js";
import { api, errorText } from "../lib/api";
import type { SessionInfo } from "../lib/types";
import { AuthShell } from "./AuthShell";

export function LoginForm(props: { notice?: string; onDone: (info: SessionInfo) => void }) {
  const [username, setUsername] = createSignal("");
  const [password, setPassword] = createSignal("");
  const [error, setError] = createSignal("");
  const [busy, setBusy] = createSignal(false);
  const [glance, setGlance] = createSignal(false);

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      props.onDone(await api.login(username(), password()));
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthShell title="Sign in to glim" glance={glance()}>
      <Show when={props.notice}>
        <p class="help" role="status">
          {props.notice}
        </p>
      </Show>
      <form class="auth-form" onSubmit={submit}>
        <div class="field">
          <label for="login-username">Username</label>
          <input
            id="login-username"
            class="input"
            autocomplete="username"
            autocapitalize="none"
            required
            value={username()}
            onInput={(e) => setUsername(e.currentTarget.value)}
          />
        </div>
        <div class="field">
          <label for="login-password">Password</label>
          <input
            id="login-password"
            class="input"
            type="password"
            autocomplete="current-password"
            required
            value={password()}
            onInput={(e) => setPassword(e.currentTarget.value)}
            onFocus={() => setGlance(true)}
            onBlur={() => setGlance(false)}
          />
        </div>
        <Show when={error()}>
          <p class="error" role="alert">
            {error()}
          </p>
        </Show>
        <button class="btn primary" type="submit" disabled={busy()}>
          Sign in
        </button>
      </form>
    </AuthShell>
  );
}

import { createMemo, createSignal, Match, onSettled, Switch } from "solid-js";
import { AuthShell } from "./components/AuthShell";
import { Dashboard } from "./components/Dashboard";
import { LoginForm } from "./components/LoginForm";
import { SetupForm } from "./components/SetupForm";
import { ApiError, api, errorText } from "./lib/api";
import type { SessionInfo, User } from "./lib/types";

type Screen =
  | { kind: "boot" }
  | { kind: "setup" }
  | { kind: "login"; notice?: string }
  | { kind: "app"; user: User }
  | { kind: "offline"; message: string };

export function App() {
  const [screen, setScreen] = createSignal<Screen>({ kind: "boot" });
  const appUser = createMemo(() => {
    const s = screen();
    return s.kind === "app" ? s.user : undefined;
  });
  const loginNotice = createMemo(() => {
    const s = screen();
    return s.kind === "login" ? (s.notice ?? "") : undefined;
  });
  const offline = createMemo(() => {
    const s = screen();
    return s.kind === "offline" ? s.message : undefined;
  });

  function signedIn(info: SessionInfo) {
    api.setCsrf(info.csrf);
    setScreen({ kind: "app", user: info.user });
  }

  function signedOut(notice?: string) {
    api.setCsrf("");
    setScreen({ kind: "login", notice });
  }

  async function signOut() {
    await api.logout().catch(() => {});
    signedOut();
  }

  async function boot() {
    try {
      const { needed } = await api.setupStatus();
      if (needed) {
        setScreen({ kind: "setup" });
        return;
      }
      signedIn(await api.session());
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) setScreen({ kind: "login" });
      else setScreen({ kind: "offline", message: errorText(e) });
    }
  }

  api.onUnauthorized(() => signedOut("Your session ended. Sign in again."));
  onSettled(() => {
    void boot();
  });

  return (
    <Switch>
      <Match when={screen().kind === "boot"}>
        <AuthShell title="glim" />
      </Match>
      <Match when={screen().kind === "setup"}>
        <SetupForm onDone={signedIn} />
      </Match>
      <Match when={loginNotice() !== undefined}>
        <LoginForm notice={loginNotice() || undefined} onDone={signedIn} />
      </Match>
      <Match when={offline()} keyed>
        {(message) => (
          <AuthShell title="Can't reach glim">
            <p class="help">{message}</p>
            <button class="btn" type="button" onClick={() => void boot()}>
              Try again
            </button>
          </AuthShell>
        )}
      </Match>
      <Match when={appUser()} keyed>
        {(user) => (
          <Dashboard
            user={user}
            onSignOut={() => void signOut()}
            onSessionEnded={() => signedOut("Your session ended. Sign in again.")}
          />
        )}
      </Match>
    </Switch>
  );
}

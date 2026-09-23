import { createEffect, createMemo, createSignal, For, onSettled, Show } from "solid-js";
import { createActions } from "../lib/actions";
import { api } from "../lib/api";
import { createClock } from "../lib/clock";
import {
  type Filters as FilterValue,
  filtersFromSearch,
  filtersToSearch,
  matches,
  projectsOf,
} from "../lib/filter";
import { createLive } from "../lib/live";
import { displayTitle } from "../lib/preview";
import { formatLeft } from "../lib/time";
import { createToasts } from "../lib/toasts";
import type { Preview, User } from "../lib/types";
import { ConfirmDialog } from "./ConfirmDialog";
import { Filters } from "./Filters";
import { PreviewCard } from "./PreviewCard";
import { StatusBar } from "./StatusBar";
import { ToastList } from "./Toasts";

export function Dashboard(props: {
  user: User;
  onSignOut: () => void;
  onSessionEnded: () => void;
}) {
  const toasts = createToasts();
  const live = createLive({ api, onUnauthorized: () => props.onSessionEnded() });
  const clock = createClock(live.skew);
  const actions = createActions({ api, live, toast: toasts.show, now: clock.now });
  const [filters, setFilters] = createSignal<FilterValue>(filtersFromSearch(location.search));
  const [confirming, setConfirming] = createSignal<Preview | null>(null);
  const [ready, setReady] = createSignal(false);

  onSettled(() => {
    const stopLive = live.start();
    const stopClock = clock.start();
    const onVisible = () => {
      if (document.visibilityState === "visible") void live.refresh();
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      stopLive();
      stopClock();
      document.removeEventListener("visibilitychange", onVisible);
    };
  });

  // Previews present at first load appear as-is; later arrivals get the blink.
  createEffect(
    () => live.state.loaded,
    (loaded) => {
      if (!loaded) return;
      const id = setTimeout(() => setReady(true), 50);
      return () => clearTimeout(id);
    },
  );

  createEffect(
    () => filtersToSearch(filters()),
    (search) => {
      history.replaceState(null, "", `${location.pathname}${search}`);
    },
  );

  const visible = createMemo(() => live.state.previews.filter((p) => matches(p, filters())));
  const projects = createMemo(() => projectsOf(live.state.previews));
  const next = createMemo(() => {
    let soonest: Preview | undefined;
    for (const p of live.state.previews) {
      if (p.pinned || actions.overrides[p.name]?.pinned) continue;
      if (!soonest || p.expires < soonest.expires) soonest = p;
    }
    if (!soonest) return undefined;
    const exp = actions.overrides[soonest.name]?.expires ?? soonest.expires;
    return { title: displayTitle(soonest), left: formatLeft(Date.parse(exp) - clock.now()) };
  });

  async function copy(p: Preview) {
    try {
      await navigator.clipboard.writeText(p.url);
      toasts.show("Link copied");
    } catch {
      toasts.show("Couldn't copy the link. Use Open and copy it from the address bar.", "error");
    }
  }

  function confirmRemove() {
    const p = confirming();
    setConfirming(null);
    if (p) void actions.remove(p.name);
  }

  return (
    <div class="dash">
      <StatusBar
        conn={live.conn()}
        status={live.state.status}
        accountLabel="Sign out"
        onAccount={() => props.onSignOut()}
      />
      <div class="toolbar">
        <Filters value={filters()} projects={projects()} onChange={setFilters} />
        <Show when={next()}>
          {(n) => (
            <p class="next">
              {n().title} fades in {n().left}
            </p>
          )}
        </Show>
      </div>
      <Show when={live.state.loaded} fallback={<p class="help pad">Loading previews…</p>}>
        <Show
          when={live.state.previews.length > 0}
          fallback={
            <p class="empty">
              Nothing to see yet. Publish a page with <code>glim page.html</code> and it appears
              here.
            </p>
          }
        >
          <Show
            when={visible().length > 0}
            fallback={
              <div class="empty">
                <p>
                  {filters().q
                    ? `No previews match "${filters().q}".`
                    : "No previews match these filters."}
                </p>
                <button
                  class="btn"
                  type="button"
                  onClick={() => setFilters({ q: "", project: "" })}
                >
                  Clear filters
                </button>
              </div>
            }
          >
            <section class="grid" aria-label="Live previews">
              <For each={visible()}>
                {(p) => (
                  <PreviewCard
                    preview={p}
                    override={actions.overrides[p.name]}
                    now={clock.now()}
                    arriving={ready()}
                    onExtend={(ttl, label) => void actions.extend(p.name, ttl, label)}
                    onPin={() => void actions.pin(p.name)}
                    onRemove={() => setConfirming(p)}
                    onCopy={() => void copy(p)}
                  />
                )}
              </For>
            </section>
          </Show>
        </Show>
      </Show>
      <ConfirmDialog
        open={confirming() !== null}
        title="Remove this preview?"
        body={
          confirming()
            ? `${displayTitle(confirming() as Preview)} is deleted now and its link stops working.`
            : ""
        }
        confirmLabel="Remove"
        onConfirm={confirmRemove}
        onCancel={() => setConfirming(null)}
      />
      <ToastList toasts={toasts} />
    </div>
  );
}

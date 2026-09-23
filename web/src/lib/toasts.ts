import { createStore } from "solid-js";

export type Toast = { id: number; text: string; tone: "info" | "error" };

export function createToasts(ttlMs = 4000) {
  const [toasts, setToasts] = createStore<{ list: Toast[] }>({ list: [] });
  let next = 1;
  function dismiss(id: number) {
    setToasts((d) => {
      const i = d.list.findIndex((t) => t.id === id);
      if (i >= 0) d.list.splice(i, 1);
    });
  }
  function show(text: string, tone: Toast["tone"] = "info") {
    const id = next++;
    setToasts((d) => {
      d.list.push({ id, text, tone });
    });
    setTimeout(() => dismiss(id), ttlMs);
  }
  return { toasts, show, dismiss };
}

export type Toasts = ReturnType<typeof createToasts>;

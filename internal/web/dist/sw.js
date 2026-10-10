// Push-only service worker. It deliberately has no "fetch" handler: previews
// live on this same origin and must never be intercepted or cached.
self.addEventListener("push", (event) => {
  let data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch {
    data = { body: event.data ? event.data.text() : "" };
  }
  const title = data.title || "glim";
  event.waitUntil(
    self.registration.showNotification(title, {
      body: data.body || "",
      icon: "/_glim/icon-192.png",
      data: { url: data.url || "/" },
    }),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  // Only same-origin URLs are opened; anything else falls back to the dashboard.
  let target = new URL("/", self.location.origin);
  try {
    const u = new URL(event.notification.data?.url || "/", self.location.origin);
    if (u.origin === self.location.origin) target = u;
  } catch {
    // keep the dashboard
  }
  target = target.href;
  event.waitUntil(
    (async () => {
      const wins = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
      for (const w of wins) {
        if (w.url === target && "focus" in w) return w.focus();
      }
      return self.clients.openWindow(target);
    })(),
  );
});

// The browser rotated or expired the subscription: fetch the key, subscribe
// again and register it using the signed-in session cookie (and its CSRF token).
// If the user is signed out this fails quietly and the toggle shows "off" the
// next time the dashboard is opened.
self.addEventListener("pushsubscriptionchange", (event) => {
  event.waitUntil(
    (async () => {
      const sess = await fetch("/_glim/api/session", { credentials: "same-origin" });
      if (!sess.ok) return;
      const { csrf } = await sess.json();
      const keyRes = await fetch("/_glim/api/push/key", { credentials: "same-origin" });
      if (!keyRes.ok) return;
      const { key } = await keyRes.json();
      const raw = atob((key + "=".repeat((4 - (key.length % 4)) % 4)).replace(/-/g, "+").replace(/_/g, "/"));
      const applicationServerKey = Uint8Array.from(raw, (c) => c.charCodeAt(0));
      const sub = await self.registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey,
      });
      const j = sub.toJSON();
      await fetch("/_glim/api/push/subscribe", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-Glim-CSRF": csrf },
        body: JSON.stringify({ endpoint: sub.endpoint, keys: j.keys }),
      });
    })().catch(() => {}),
  );
});

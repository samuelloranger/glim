import type { Api } from "./api";

export type PushState = "unsupported" | "denied" | "off" | "on";

/** The browser surface push needs; injectable so the logic is testable. */
export interface PushEnv {
  hasPushManager: boolean;
  permission: () => NotificationPermission;
  requestPermission: () => Promise<NotificationPermission>;
  /** Registers (or finds) the push-only service worker. */
  registration: () => Promise<ServiceWorkerRegistration | undefined>;
}

export const SW_URL = "/_glim/sw.js";

export function browserPushEnv(): PushEnv {
  const supported =
    typeof window !== "undefined" &&
    "PushManager" in window &&
    "Notification" in window &&
    "serviceWorker" in navigator;
  return {
    hasPushManager: supported,
    permission: () => Notification.permission,
    requestPermission: () => Notification.requestPermission(),
    registration: async () => {
      const existing = await navigator.serviceWorker.getRegistration("/");
      if (existing) return existing;
      return navigator.serviceWorker.register(SW_URL, { scope: "/" });
    },
  };
}

/** Decodes the URL-safe base64 VAPID public key into applicationServerKey bytes. */
export function urlBase64ToBytes(s: string): Uint8Array<ArrayBuffer> {
  const pad = "=".repeat((4 - (s.length % 4)) % 4);
  const raw = atob((s + pad).replace(/-/g, "+").replace(/_/g, "/"));
  const out = new Uint8Array(new ArrayBuffer(raw.length));
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}

export async function pushState(env: PushEnv): Promise<PushState> {
  if (!env.hasPushManager) return "unsupported";
  if (env.permission() === "denied") return "denied";
  try {
    const reg = await env.registration();
    const sub = await reg?.pushManager.getSubscription();
    return sub && env.permission() === "granted" ? "on" : "off";
  } catch {
    return "off";
  }
}

type PushApi = Pick<Api, "pushKey" | "pushSubscribe" | "pushUnsubscribe">;

/** Must run from a click: iOS only shows the permission prompt on a user gesture. */
export async function enablePush(env: PushEnv, api: PushApi): Promise<PushState> {
  if (!env.hasPushManager) return "unsupported";
  const perm = await env.requestPermission();
  if (perm === "denied") return "denied";
  if (perm !== "granted") return "off";
  const reg = await env.registration();
  if (!reg) return "unsupported";
  const { key } = await api.pushKey();
  const sub =
    (await reg.pushManager.getSubscription()) ??
    (await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlBase64ToBytes(key),
    }));
  const json = sub.toJSON();
  await api.pushSubscribe({
    endpoint: sub.endpoint,
    keys: { p256dh: json.keys?.p256dh ?? "", auth: json.keys?.auth ?? "" },
  });
  return "on";
}

export async function disablePush(env: PushEnv, api: PushApi): Promise<PushState> {
  const reg = await env.registration();
  const sub = await reg?.pushManager.getSubscription();
  if (sub) {
    await api.pushUnsubscribe(sub.endpoint);
    await sub.unsubscribe();
  }
  return env.permission() === "denied" ? "denied" : "off";
}

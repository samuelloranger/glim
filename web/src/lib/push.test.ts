import { expect, test } from "bun:test";
import { createApi } from "./api";
import { disablePush, enablePush, type PushEnv, pushState, urlBase64ToBytes } from "./push";

function fakeSub(endpoint = "https://push.example/abc") {
  let removed = false;
  return {
    endpoint,
    toJSON: () => ({ endpoint, keys: { p256dh: "P", auth: "A" } }),
    unsubscribe: async () => {
      removed = true;
      return true;
    },
    get removed() {
      return removed;
    },
  };
}

function fakeEnv(opts: {
  push?: boolean;
  permission?: NotificationPermission;
  ask?: NotificationPermission;
  existing?: ReturnType<typeof fakeSub> | null;
}) {
  let sub = opts.existing ?? null;
  const subscribeCalls: unknown[] = [];
  const env: PushEnv = {
    hasPushManager: opts.push ?? true,
    permission: () => opts.permission ?? "default",
    requestPermission: async () => opts.ask ?? "granted",
    registration: async () =>
      ({
        pushManager: {
          getSubscription: async () => sub,
          subscribe: async (o: unknown) => {
            subscribeCalls.push(o);
            sub = fakeSub();
            return sub;
          },
        },
      }) as unknown as ServiceWorkerRegistration,
  };
  return { env, subscribeCalls };
}

function fakeApi() {
  const calls: { url: string; body: unknown }[] = [];
  const fn = (async (url: string, init: RequestInit) => {
    calls.push({ url, body: init.body ? JSON.parse(String(init.body)) : undefined });
    if (url.endsWith("/push/key")) return new Response(JSON.stringify({ key: "BAAA" }));
    return new Response(null, { status: 204 });
  }) as unknown as typeof fetch;
  return { api: createApi(fn), calls };
}

test("state is unsupported without PushManager", async () => {
  expect(await pushState(fakeEnv({ push: false }).env)).toBe("unsupported");
});

test("state is denied when permission is denied", async () => {
  expect(await pushState(fakeEnv({ permission: "denied" }).env)).toBe("denied");
});

test("state is off without a subscription and on with one", async () => {
  expect(await pushState(fakeEnv({ permission: "granted" }).env)).toBe("off");
  expect(await pushState(fakeEnv({ permission: "granted", existing: fakeSub() }).env)).toBe("on");
});

test("enable asks permission, subscribes with the VAPID key and posts it", async () => {
  const { env, subscribeCalls } = fakeEnv({});
  const { api, calls } = fakeApi();
  expect(await enablePush(env, api)).toBe("on");
  expect((subscribeCalls[0] as { userVisibleOnly: boolean }).userVisibleOnly).toBe(true);
  expect(calls.map((c) => c.url)).toEqual(["/_glim/api/push/key", "/_glim/api/push/subscribe"]);
  expect(calls[1]?.body).toEqual({
    endpoint: "https://push.example/abc",
    keys: { p256dh: "P", auth: "A" },
  });
});

test("enable stops when permission is denied", async () => {
  const { env, subscribeCalls } = fakeEnv({ ask: "denied" });
  const { api, calls } = fakeApi();
  expect(await enablePush(env, api)).toBe("denied");
  expect(subscribeCalls.length).toBe(0);
  expect(calls.length).toBe(0);
});

test("enable reports unsupported without PushManager", async () => {
  expect(await enablePush(fakeEnv({ push: false }).env, fakeApi().api)).toBe("unsupported");
});

test("disable unsubscribes on the server and in the browser", async () => {
  const sub = fakeSub();
  const { env } = fakeEnv({ permission: "granted", existing: sub });
  const { api, calls } = fakeApi();
  expect(await disablePush(env, api)).toBe("off");
  expect(calls[0]?.url).toBe("/_glim/api/push/unsubscribe");
  expect(calls[0]?.body).toEqual({ endpoint: "https://push.example/abc" });
  expect(sub.removed).toBe(true);
});

test("urlBase64ToBytes decodes URL-safe base64", () => {
  expect(Array.from(urlBase64ToBytes("-_8"))).toEqual([251, 255]);
});

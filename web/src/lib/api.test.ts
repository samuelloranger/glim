import { expect, test } from "bun:test";
import { ApiError, createApi } from "./api";

type Call = { url: string; init: RequestInit };

function fakeFetch(status: number, body: unknown, headers: Record<string, string> = {}) {
  const calls: Call[] = [];
  const fn = (async (url: string, init: RequestInit) => {
    calls.push({ url, init });
    return new Response(status === 204 ? null : JSON.stringify(body), { status, headers });
  }) as unknown as typeof fetch;
  return { fn, calls };
}

test("mutations send JSON and the CSRF token", async () => {
  const f = fakeFetch(200, { name: "a" });
  const api = createApi(f.fn);
  api.setCsrf("tok");
  await api.extend("diff-a1b2", "6h");
  const call = f.calls[0];
  expect(call?.url).toBe("/_glim/api/previews/diff-a1b2/extend");
  const h = call?.init.headers as Record<string, string>;
  expect(h["X-Glim-CSRF"]).toBe("tok");
  expect(h["Content-Type"]).toBe("application/json");
  expect(call?.init.body).toBe(JSON.stringify({ ttl: "6h" }));
});

test("errors become ApiError with server message and Retry-After", async () => {
  const f = fakeFetch(
    429,
    { error: "Too many attempts.", code: "rate_limited" },
    { "Retry-After": "7" },
  );
  const api = createApi(f.fn);
  const err = await api.login("sam", "x").catch((e: unknown) => e);
  expect(err).toBeInstanceOf(ApiError);
  expect((err as ApiError).code).toBe("rate_limited");
  expect((err as ApiError).message).toBe("Too many attempts.");
  expect((err as ApiError).retryAfter).toBe(7);
});

test("401 signs out, except on session/login/setup", async () => {
  let signedOut = 0;
  const f = fakeFetch(401, { error: "Sign in to continue.", code: "unauthorized" });
  const api = createApi(f.fn);
  api.onUnauthorized(() => signedOut++);
  await api.session().catch(() => {});
  await api.login("sam", "x").catch(() => {});
  expect(signedOut).toBe(0);
  await api.previews().catch(() => {});
  expect(signedOut).toBe(1);
});

test("network failure is a readable ApiError", async () => {
  const api = createApi((async () => {
    throw new TypeError("fetch failed");
  }) as unknown as typeof fetch);
  const err = (await api.previews().catch((e: unknown) => e)) as ApiError;
  expect(err.status).toBe(0);
  expect(err.message).toContain("Can't reach the glim server");
});

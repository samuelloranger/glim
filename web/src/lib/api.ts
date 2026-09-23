import type { ApiErrorCode, Preview, SessionInfo, Snapshot, User } from "./types";

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: ApiErrorCode,
    message: string,
    readonly retryAfter?: number,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

// A 401 from these means "not signed in yet", not "your session just ended".
const NO_SIGN_OUT = ["/session", "/login", "/setup"];

export function createApi(fetchFn: typeof fetch = fetch) {
  let csrf = "";
  let unauthorized = () => {};

  async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = {};
    if (body !== undefined) headers["Content-Type"] = "application/json";
    if (method !== "GET") headers["X-Glim-CSRF"] = csrf;
    let res: Response;
    try {
      res = await fetchFn(`/_glim/api${path}`, {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
        credentials: "same-origin",
      });
    } catch {
      throw new ApiError(0, "internal", "Can't reach the glim server. Check your connection.");
    }
    if (res.status === 204) return undefined as T;
    const data = (await res.json().catch(() => null)) as {
      error?: string;
      code?: ApiErrorCode;
    } | null;
    if (!res.ok) {
      if (res.status === 401 && !NO_SIGN_OUT.some((p) => path.startsWith(p))) unauthorized();
      const retry = Number(res.headers.get("Retry-After"));
      throw new ApiError(
        res.status,
        data?.code ?? "internal",
        data?.error ?? `Request failed (${res.status}).`,
        retry > 0 ? retry : undefined,
      );
    }
    return data as T;
  }

  const enc = encodeURIComponent;
  return {
    setCsrf(token: string) {
      csrf = token;
    },
    onUnauthorized(fn: () => void) {
      unauthorized = fn;
    },
    setupStatus: () => request<{ needed: boolean }>("GET", "/setup"),
    setup: (code: string, username: string, password: string) =>
      request<SessionInfo>("POST", "/setup", { code, username, password }),
    login: (username: string, password: string) =>
      request<SessionInfo>("POST", "/login", { username, password }),
    logout: () => request<void>("POST", "/logout"),
    session: () => request<SessionInfo>("GET", "/session"),
    previews: () => request<Snapshot>("GET", "/previews"),
    extend: (name: string, ttl: string) =>
      request<Preview>("POST", `/previews/${enc(name)}/extend`, { ttl }),
    pin: (name: string) => request<Preview>("POST", `/previews/${enc(name)}/pin`),
    remove: (name: string) => request<void>("DELETE", `/previews/${enc(name)}`),
    users: () => request<{ users: User[] }>("GET", "/users"),
    addUser: (username: string, password: string) =>
      request<User>("POST", "/users", { username, password }),
    removeUser: (username: string) => request<void>("DELETE", `/users/${enc(username)}`),
    changePassword: (current: string, next: string) =>
      request<void>("POST", "/account/password", { current, next }),
  };
}

export type Api = ReturnType<typeof createApi>;
export const api = createApi();

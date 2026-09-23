export type Preview = {
  name: string;
  title: string;
  project: string;
  created: string;
  expires: string;
  pinned: boolean;
  url: string;
};

export type Status = {
  live: number;
  pinned: number;
  diskBytes: number;
  nextExpiry: string | null;
};

export type Snapshot = { now: string; previews: Preview[]; status: Status };

export type User = { username: string; createdAt: string };

export type SessionInfo = { user: User; csrf: string };

export type ApiErrorCode =
  | "not_found"
  | "invalid"
  | "unauthorized"
  | "forbidden"
  | "conflict"
  | "rate_limited"
  | "internal";

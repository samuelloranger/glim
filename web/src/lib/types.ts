export type Preview = {
  name: string;
  title: string;
  project: string;
  created: string;
  expires: string;
  pinned: boolean;
  locked: boolean;
  url: string;
  views: number;
  lastSeen: string | null;
  /** Unresolved comments left from the glim bar. */
  comments: number;
};

export type Status = {
  live: number;
  pinned: number;
  diskBytes: number;
  nextExpiry: string | null;
};

export type Snapshot = { now: string; previews: Preview[]; status: Status };

export type User = { email: string; createdAt: string };

export type SessionInfo = { user: User; csrf: string };

export type ApiErrorCode =
  | "not_found"
  | "invalid"
  | "unauthorized"
  | "forbidden"
  | "conflict"
  | "rate_limited"
  | "internal";

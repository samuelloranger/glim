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

/** Optional server features: Listen makes audio versions; Speech is false
 * when the server writes the narration but leaves reading it to the device. */
export type Features = { listen: boolean; speech: boolean };

export type SessionInfo = { user: User; csrf: string; features?: Features };

export type ListenResult = {
  version: string;
  created: string;
  language?: string;
  summary: string;
  narration: string;
  audio: boolean;
  seconds?: number;
  model: string;
  voice?: string;
  /** False when the page was read from its HTML, without screenshots. */
  rendered: boolean;
  pictures: number;
};

export type ListenStep = "queued" | "page" | "script" | "audio";

export type ListenStatus = {
  state: "none" | "working" | "ready" | "failed";
  step?: ListenStep;
  done?: number;
  total?: number;
  error?: string;
  result?: ListenResult;
  /** The preview was republished after result was made. */
  stale: boolean;
  speech: boolean;
};

export type ApiErrorCode =
  | "not_found"
  | "invalid"
  | "unauthorized"
  | "forbidden"
  | "conflict"
  | "rate_limited"
  | "internal";

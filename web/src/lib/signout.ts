import type { Api } from "./api";

export type SignedOutScreen = { kind: "setup" } | { kind: "login"; notice?: string };

/**
 * Where a signed-out tab goes. If the last account was removed while the tab was
 * open there is nobody to sign in as, so offer to create the first one instead.
 * When the check fails (offline), sign-in is the safe default.
 */
export async function screenAfterSignOut(
  api: Pick<Api, "setupStatus">,
  notice?: string,
): Promise<SignedOutScreen> {
  try {
    if ((await api.setupStatus()).needed) return { kind: "setup" };
  } catch {
    // fall through to sign-in
  }
  return { kind: "login", notice };
}

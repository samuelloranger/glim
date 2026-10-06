import { expect, test } from "bun:test";
import { screenAfterSignOut } from "./signout";

test("last account removed -> setup form, not sign-in", async () => {
  const api = { setupStatus: async () => ({ needed: true }) };
  expect(await screenAfterSignOut(api, "Your session ended.")).toEqual({ kind: "setup" });
});

test("accounts remain -> sign-in with the notice", async () => {
  const api = { setupStatus: async () => ({ needed: false }) };
  expect(await screenAfterSignOut(api, "Your session ended.")).toEqual({
    kind: "login",
    notice: "Your session ended.",
  });
});

test("setup check failing falls back to sign-in", async () => {
  const api = {
    setupStatus: async () => {
      throw new Error("offline");
    },
  };
  expect(await screenAfterSignOut(api)).toEqual({ kind: "login", notice: undefined });
});

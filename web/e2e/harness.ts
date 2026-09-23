import { execFileSync, spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

export const PORT = 18799;

async function waitForServer() {
  const deadline = Date.now() + 15_000;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`http://127.0.0.1:${PORT}/_glim/api/setup`);
      if (res.ok) return;
    } catch {
      // not up yet
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error("glim serve did not start");
}

export default async function globalSetup() {
  const home = mkdtempSync(join(tmpdir(), "glim-e2e-"));
  const bin = join(home, "glim");
  const repo = fileURLToPath(new URL("../..", import.meta.url));
  execFileSync("go", ["build", "-o", bin, "."], { cwd: repo, stdio: "inherit" });
  // Default root ($HOME/.glim/pub) so CLI publishes in util.ts land where the server looks.
  const server = spawn(bin, ["serve", "--port", String(PORT)], {
    env: { ...process.env, HOME: home },
    stdio: "inherit",
  });
  await waitForServer();
  process.env.GLIM_E2E_HOME = home;
  process.env.GLIM_E2E_BIN = bin;
  return async () => {
    server.kill();
    rmSync(home, { recursive: true, force: true });
  };
}

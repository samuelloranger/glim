import { execFileSync, spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

export const PORT = 18799;
const PROVIDER_PORT = 18798;

/** Two MPEG-1 layer III frames: what a speech API returns, in miniature. */
function mp3(): Buffer {
  const frame = Buffer.alloc(417);
  frame.set([0xff, 0xfb, 0x90, 0x00]);
  return Buffer.concat([frame, frame]);
}

/**
 * Stands in for an OpenAI-compatible model and speech API, so Listen runs
 * without a network or a GPU.
 */
function fakeProviders() {
  const srv = createServer((req, res) => {
    req.resume();
    req.on("end", () => {
      if (req.url === "/v1/chat/completions") {
        const content = JSON.stringify({
          language: "en",
          summary: "A short test page.",
          narration: "This page is a short test page with a heading.",
        });
        res.setHeader("Content-Type", "application/json");
        res.end(JSON.stringify({ choices: [{ message: { content } }] }));
      } else if (req.url === "/v1/audio/speech") {
        res.setHeader("Content-Type", "audio/mpeg");
        res.end(mp3());
      } else {
        res.statusCode = 404;
        res.end();
      }
    });
  });
  srv.listen(PROVIDER_PORT, "127.0.0.1");
  return srv;
}

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
  const providers = fakeProviders();
  const provider = `http://127.0.0.1:${PROVIDER_PORT}/v1`;
  const server = spawn(bin, ["serve", "--port", String(PORT)], {
    env: {
      ...process.env,
      HOME: home,
      GLIM_LLM_URL: provider,
      GLIM_LLM_MODEL: "test-vision",
      GLIM_TTS_URL: provider,
      GLIM_TTS_MODEL: "test-speech",
      GLIM_CHROME: "off",
    },
    stdio: "inherit",
  });
  await waitForServer();
  process.env.GLIM_E2E_HOME = home;
  process.env.GLIM_E2E_BIN = bin;
  return async () => {
    server.kill();
    providers.close();
    rmSync(home, { recursive: true, force: true });
  };
}

import { execFileSync, spawn } from "node:child_process";
import { writeFileSync } from "node:fs";
import { join } from "node:path";

const home = () => process.env.GLIM_E2E_HOME as string;
const bin = () => process.env.GLIM_E2E_BIN as string;

/** Publishes like an agent would (a separate CLI process); returns the slug. */
export function publish(
  title: string,
  opts: { project?: string; ttl?: string; name?: string; body?: string } = {},
): string {
  const file = join(home(), `${Date.now()}-${Math.random().toString(36).slice(2)}.html`);
  writeFileSync(file, opts.body ?? `<h1>${title}</h1>`);
  const args = [file, "--title", title, "--ttl", opts.ttl ?? "6h"];
  if (opts.project) args.push("--project", opts.project);
  if (opts.name) args.push("--name", opts.name);
  const url = execFileSync(bin(), args, { env: { ...process.env, HOME: home() } })
    .toString()
    .trim();
  return new URL(url).pathname.replaceAll("/", "");
}

/**
 * Calls one tool of `glim mcp` over stdio, like an agent would, and returns
 * the tool result's structured content.
 */
export function mcpCall(
  tool: string,
  args: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  const proc = spawn(bin(), ["mcp"], { env: { ...process.env, HOME: home() } });
  const send = (msg: unknown) => proc.stdin.write(`${JSON.stringify(msg)}\n`);
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      proc.kill();
      reject(new Error(`glim mcp: no answer to ${tool}`));
    }, 10_000);
    let buf = "";
    proc.stdout.on("data", (chunk: Buffer) => {
      buf += chunk.toString();
      for (let i = buf.indexOf("\n"); i >= 0; i = buf.indexOf("\n")) {
        const line = buf.slice(0, i);
        buf = buf.slice(i + 1);
        const msg = JSON.parse(line) as {
          id?: number;
          result?: Record<string, unknown>;
          error?: unknown;
        };
        if (msg.id === 1) {
          send({ jsonrpc: "2.0", method: "notifications/initialized" });
          send({
            jsonrpc: "2.0",
            id: 2,
            method: "tools/call",
            params: { name: tool, arguments: args },
          });
        } else if (msg.id === 2) {
          clearTimeout(timer);
          proc.kill();
          const result = msg.result as {
            isError?: boolean;
            structuredContent?: Record<string, unknown>;
          };
          if (msg.error || result?.isError) reject(new Error(JSON.stringify(msg.error ?? result)));
          else resolve(result.structuredContent ?? {});
        }
      }
    });
    send({
      jsonrpc: "2.0",
      id: 1,
      method: "initialize",
      params: {
        protocolVersion: "2025-06-18",
        capabilities: {},
        clientInfo: { name: "e2e", version: "1" },
      },
    });
  });
}

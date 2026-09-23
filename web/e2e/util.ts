import { execFileSync } from "node:child_process";
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

import { type BrowserContext, expect, type Page, test } from "@playwright/test";
import { publish, setupCode } from "./util";

const PASSWORD = "correct horse battery";

test.describe.configure({ mode: "serial" });

let context: BrowserContext;
let page: Page;

test.beforeAll(async ({ browser }) => {
  context = await browser.newContext();
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  page = await context.newPage();
});

test.afterAll(async () => {
  await context.close();
});

const card = (title: string) => page.getByRole("article", { name: title, exact: true });

test("setup requires the code, then lands on an empty dashboard", async () => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Create your account" })).toBeVisible();
  await page.getByLabel("Setup code").fill("WRONGWRONG");
  await page.getByLabel("Username").fill("sam");
  await page.getByLabel("Password", { exact: true }).fill(PASSWORD);
  await page.getByLabel("Confirm password").fill(PASSWORD);
  await page.getByRole("button", { name: "Create account" }).click();
  await expect(page.getByRole("alert")).toContainText("setup code doesn't match");
  await page.getByLabel("Setup code").fill(setupCode());
  await page.getByRole("button", { name: "Create account" }).click();
  await expect(page.getByText("Nothing to see yet.")).toBeVisible();
});

test("a CLI publish appears live, sandboxed and lazy", async () => {
  const slug = publish("Diff review", { project: "api" });
  await expect(card("Diff review")).toBeVisible({ timeout: 5000 });
  const frame = card("Diff review").locator("iframe");
  await expect(frame).toHaveAttribute("sandbox", "allow-scripts");
  await expect(frame).toHaveAttribute("loading", "lazy");
  await expect(card("Diff review").getByRole("meter", { name: "Time left" })).toBeVisible();
  await expect(card("Diff review").getByText(slug)).toBeVisible();
  const res = await page.request.get(`/${slug}/`);
  expect(res.headers()["content-security-policy"]).toBe(
    "sandbox allow-scripts allow-forms allow-popups allow-modals allow-downloads",
  );
});

test("extend, pin and copy update in place", async () => {
  const c = card("Diff review");
  await c.hover();
  await c.getByRole("button", { name: "Extend" }).click();
  await c.getByRole("button", { name: "24h" }).click();
  await expect(page.getByText("Extended to 24h")).toBeVisible();
  await c.getByRole("button", { name: "Pin" }).click();
  await expect(page.getByText("Pinned", { exact: true })).toBeVisible();
  await expect(c.getByText("pinned", { exact: true })).toBeVisible();
  await expect(c.getByRole("button", { name: "Pin" })).toHaveCount(0);
  await c.getByRole("button", { name: "Copy link" }).click();
  await expect(page.getByText("Link copied")).toBeVisible();
});

test("republishing in place refreshes the thumbnail", async () => {
  const slug = publish("Report", {});
  const frame = card("Report").locator("iframe");
  await expect(frame).toBeVisible({ timeout: 5000 });
  const before = await frame.getAttribute("src");
  await page.waitForTimeout(50); // guarantee a distinct `created` timestamp
  publish("Report", { name: slug, body: "<h1>v2</h1>" });
  await expect.poll(async () => frame.getAttribute("src"), { timeout: 5000 }).not.toBe(before);
});

test("filters narrow the grid and live in the URL", async () => {
  publish("Login flow", { project: "web" });
  await expect(card("Login flow")).toBeVisible({ timeout: 5000 });
  await page.getByLabel("Search previews").fill("login");
  await expect(page).toHaveURL(/\?q=login/);
  await expect(card("Diff review")).toHaveCount(0);
  await page.getByLabel("Search previews").fill("zzz");
  await expect(page.getByText('No previews match "zzz".')).toBeVisible();
  await page.getByRole("button", { name: "Clear filters" }).click();
  await expect(card("Diff review")).toBeVisible();
});

test("remove asks first, then closes the preview", async () => {
  const c = card("Login flow");
  await c.hover();
  await c.getByRole("button", { name: "Remove" }).click();
  await expect(page.getByRole("dialog", { name: "Remove this preview?" })).toBeVisible();
  await page.getByRole("dialog").getByRole("button", { name: "Remove" }).click();
  await expect(page.getByText(/^Removed login-flow-/)).toBeVisible();
  await expect(card("Login flow")).toHaveCount(0);
});

test("the API refuses anonymous requests", async ({ request }) => {
  const res = await request.get("/_glim/api/previews");
  expect(res.status()).toBe(401);
});

test("sign out and back in", async () => {
  await page.getByRole("button", { name: "sam", exact: true }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Sign out" }).click();
  await expect(page.getByRole("heading", { name: "Sign in to glim" })).toBeVisible();
  await page.getByLabel("Username").fill("sam");
  await page.getByLabel("Password").fill(PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(card("Diff review")).toBeVisible();
});

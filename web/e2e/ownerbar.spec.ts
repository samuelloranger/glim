import { type BrowserContext, expect, type Page, test } from "@playwright/test";
import { publish } from "./util";

const PASSWORD = "correct horse battery";
const EMAIL = "sam@example.com";

// Every page rule tries to hide or recolour whatever it can reach, host
// element included; only the one div is meant to stay visible.
const HOSTILE = `<!doctype html><html><head><style>
  * { display: none !important; color: red !important; visibility: hidden !important; }
  html, body, #keep { display: block !important; visibility: visible !important; }
  div { position: fixed !important; inset: 0 !important; z-index: 2147483647 !important; background: #fff !important; }
  glim-bar, glim-bar * { display: none !important; }
</style></head><body><div id="keep">hostile page</div></body></html>`;

let owner: BrowserContext;
let ownerPage: Page;

test.beforeAll(async ({ browser }) => {
  owner = await browser.newContext({ viewport: { width: 1000, height: 700 } });
  // The shadow root is closed for the page; open it for the test so locators can
  // reach inside. The closed-mode behaviour is what the hostile-CSS test relies on
  // for the host element, which stays in the light DOM either way.
  await owner.addInitScript(() => {
    const attach = Element.prototype.attachShadow;
    Element.prototype.attachShadow = function (init) {
      return attach.call(this, { ...init, mode: "open" });
    };
  });
  ownerPage = await owner.newPage();
  await ownerPage.goto("/");
  if (await ownerPage.getByRole("heading", { name: "Create your account" }).isVisible()) {
    await ownerPage.getByLabel("Email").fill(EMAIL);
    await ownerPage.getByLabel("Password", { exact: true }).fill(PASSWORD);
    await ownerPage.getByLabel("Confirm password").fill(PASSWORD);
    await ownerPage.getByRole("button", { name: "Create account" }).click();
  } else if (await ownerPage.getByRole("heading", { name: "Sign in to glim" }).isVisible()) {
    await ownerPage.getByLabel("Email").fill(EMAIL);
    await ownerPage.getByLabel("Password").fill(PASSWORD);
    await ownerPage.getByRole("button", { name: "Sign in" }).click();
  }
  await expect(ownerPage.getByRole("heading", { name: "Sign in to glim" })).toHaveCount(0);
});

test.afterAll(async () => {
  await owner.close();
});

test("the owner sees the bar over a hostile page, and it collapses and expands", async () => {
  const slug = publish("Bar demo", { body: HOSTILE });
  await ownerPage.goto(`/${slug}/`);
  const host = ownerPage.locator("glim-bar");
  await expect(host).toHaveCount(1);
  await expect(host).toHaveAttribute("data-state", "expanded");

  const box = await host.boundingBox();
  expect(box).not.toBeNull();
  expect(box?.height).toBeGreaterThan(20);
  expect(box?.height).toBeLessThan(80);
  // Sits at the bottom of the viewport.
  expect((box?.y ?? 0) + (box?.height ?? 0)).toBeGreaterThan(690);

  await expect(host.getByText("Bar demo", { exact: true })).toBeVisible();
  await expect(host.getByRole("button", { name: "Copy link" })).toBeVisible();
  const dash = host.getByRole("link", { name: "Back to dashboard" });
  await expect(dash).toBeVisible();
  await expect(dash).toHaveAttribute("href", /^http:\/\/127\.0\.0\.1:\d+\/$/);

  await host.getByRole("button", { name: "Collapse glim bar" }).click();
  await expect(host).toHaveAttribute("data-state", "collapsed");
  const pill = host.getByRole("button", { name: "Show glim bar" });
  await expect(pill).toBeVisible();
  const pillBox = await pill.boundingBox();
  expect(pillBox?.width).toBeLessThan(150);
  expect((pillBox?.x ?? 0) + (pillBox?.width ?? 0)).toBeGreaterThan(900);

  await pill.click();
  await expect(host).toHaveAttribute("data-state", "expanded");
  await expect(host.getByText("Bar demo", { exact: true })).toBeVisible();
});

test("copy link gives feedback or falls back to a selected link", async () => {
  const slug = publish("Copy demo", { body: "<h1>copy</h1>" });
  await ownerPage.goto(`/${slug}/`);
  const host = ownerPage.locator("glim-bar");
  await host.getByRole("button", { name: "Copy link" }).click();
  // The sandbox may deny the clipboard; then the bar shows the link to copy by hand.
  const copied = host.getByText("Copied", { exact: true });
  const manual = host.getByRole("textbox", { name: "Preview link" });
  await expect(copied.or(manual)).toBeVisible();
  if (await manual.isVisible()) await expect(manual).toHaveValue(new RegExp(`/${slug}/$`));
});

test("the page's own layout is not shifted by the bar", async () => {
  const slug = publish("Layout demo", { body: "<body style='margin:0'><h1 id=h>top</h1></body>" });
  await ownerPage.goto(`/${slug}/`);
  await expect(ownerPage.locator("glim-bar")).toHaveCount(1);
  const pad = await ownerPage.evaluate(() => getComputedStyle(document.body).paddingBottom);
  expect(pad).toBe("0px");
  const h = await ownerPage.locator("#h").boundingBox();
  expect(h?.y).toBeLessThan(40);
});

test("an anonymous visitor never gets the bar", async ({ browser }) => {
  const slug = publish("Anon demo", { body: "<h1>plain</h1>" });
  const visitor = await browser.newContext();
  try {
    const page = await visitor.newPage();
    await page.goto(`/${slug}/`);
    await expect(page.getByRole("heading", { name: "plain" })).toBeVisible();
    await expect(page.locator("glim-bar")).toHaveCount(0);
    const html = await (await page.request.get(`/${slug}/`)).text();
    expect(html).not.toContain("glim-bar");
  } finally {
    await visitor.close();
  }
});

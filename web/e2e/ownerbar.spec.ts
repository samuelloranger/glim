import { type BrowserContext, expect, type Page, test } from "@playwright/test";
import { mcpCall, publish } from "./util";

const PASSWORD = "correct horse battery";
const EMAIL = "sam@example.com";

// Every page rule tries to hide or recolour whatever it can reach, host
// element included; only the one div is meant to stay visible.
const HOSTILE = `<!doctype html><html><head><style>
  * { display: none !important; color: red !important; visibility: hidden !important; }
  html, body, #keep { display: block !important; visibility: visible !important; }
  div { position: fixed !important; inset: 0 !important; z-index: 2147483647 !important; background: #fff !important; }
  glim-bar, glim-bar *, [data-glim-bar], [data-glim-bar] * { display: none !important; }
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
  const host = ownerPage.locator("[data-glim-bar]");
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

test("a page that uses the glim-bar name itself still gets the bar", async () => {
  const slug = publish("Name clash demo", {
    body: `<glim-bar>page element</glim-bar><script>
      customElements.define("glim-bar", class extends HTMLElement {
        constructor() { super(); this.attachShadow({ mode: "open" }); }
      });
    </script>`,
  });
  await ownerPage.goto(`/${slug}/`);
  const host = ownerPage.locator("[data-glim-bar]");
  await expect(host).toHaveCount(1);
  await expect(host.getByText("Name clash demo", { exact: true })).toBeVisible();
});

test("on a phone-width screen the bar docks flush to the bottom edge", async () => {
  const slug = publish("Dock demo", {
    body: "<meta name=viewport content='width=device-width'><h1>dock</h1>",
  });
  await ownerPage.setViewportSize({ width: 390, height: 700 });
  try {
    await ownerPage.goto(`/${slug}/`);
    const host = ownerPage.locator("[data-glim-bar]");
    const bar = host.getByText("Dock demo", { exact: true }).locator("..");
    const box = await bar.boundingBox();
    expect(box?.x).toBe(0);
    expect(box?.width).toBe(390);
    expect((box?.y ?? 0) + (box?.height ?? 0)).toBe(700);
    expect(box?.height).toBeLessThanOrEqual(44);
    const vp = await ownerPage.locator('meta[name="viewport"]').getAttribute("content");
    expect(vp).toContain("viewport-fit=cover");
  } finally {
    await ownerPage.setViewportSize({ width: 1000, height: 700 });
  }
});

test("copy link gives feedback or falls back to a selected link", async () => {
  const slug = publish("Copy demo", { body: "<h1>copy</h1>" });
  await ownerPage.goto(`/${slug}/`);
  const host = ownerPage.locator("[data-glim-bar]");
  await host.getByRole("button", { name: "Copy link" }).click();
  // The sandbox may deny the clipboard; then the bar shows the link to copy by hand.
  const copied = host.getByText("Copied", { exact: true });
  const manual = host.getByRole("textbox", { name: "Preview link" });
  await expect(copied.or(manual)).toBeVisible();
  if (await manual.isVisible()) await expect(manual).toHaveValue(new RegExp(`/${slug}/$`));
});

test("share opens the link as a QR code above the bar, and Escape closes it", async () => {
  const slug = publish("Share demo", { body: "<h1>share</h1>" });
  await ownerPage.goto(`/${slug}/`);
  const host = ownerPage.locator("[data-glim-bar]");
  const share = host.getByRole("button", { name: "Share link" });
  await share.click();
  await expect(share).toHaveAttribute("aria-expanded", "true");
  const panel = host.getByRole("group", { name: "Share link" });
  await expect(panel.getByRole("img", { name: "QR code of the link" })).toBeVisible();
  await expect(panel.getByText(new RegExp(`/${slug}/$`))).toBeVisible();
  const box = await panel.getByRole("img", { name: "QR code of the link" }).boundingBox();
  expect(box?.width).toBeGreaterThan(150);
  await ownerPage.keyboard.press("Escape");
  await expect(panel).toHaveCount(0);
  await expect(share).toHaveAttribute("aria-expanded", "false");
});

test("a comment pinned to an element reaches the agent, and its reply comes back", async () => {
  const slug = publish("Comment demo", {
    body: `<main><h1 id="t" style="margin:40px">Quarterly plan</h1><p>Body text</p></main>
      <script>addEventListener("click", () => { document.title = "page saw the click"; });</script>`,
  });
  await ownerPage.goto(`/${slug}/`);
  const host = ownerPage.locator("[data-glim-bar]");
  await host.getByRole("button", { name: "Comments" }).click();
  const list = host.getByRole("group", { name: "Comments" });
  await expect(list.getByText("No comments yet.", { exact: false })).toBeVisible();

  await list.getByRole("button", { name: "Add" }).click();
  await expect(host.getByRole("group", { name: "Add a comment" })).toBeVisible();
  // Escape leaves pick mode without the page seeing the key.
  await ownerPage.evaluate(() =>
    addEventListener("keydown", () => {
      document.title = "page saw the key";
    }),
  );
  await ownerPage.keyboard.press("Escape");
  await expect(list).toBeVisible();
  expect(await ownerPage.title()).not.toBe("page saw the key");
  await list.getByRole("button", { name: "Add" }).click();
  await ownerPage.locator("#t").click({ position: { x: 10, y: 10 } });
  // Picking swallows the click: the page's own handler never runs.
  expect(await ownerPage.title()).not.toBe("page saw the click");
  const composer = host.getByRole("group", { name: "New comment" });
  await expect(composer.getByText("On “Quarterly plan”")).toBeVisible();
  await composer.getByRole("textbox", { name: "Comment" }).fill("Make the title bigger");
  await composer.getByRole("button", { name: "Comment", exact: true }).click();

  await expect(list.getByText("Make the title bigger")).toBeVisible();
  await expect(host.getByRole("button", { name: "Comments (1 open)" })).toBeVisible();
  const pin = host.getByRole("button", { name: "Comment 1" });
  await expect(pin).toBeVisible();
  const [p, h] = [await pin.boundingBox(), await ownerPage.locator("#t").boundingBox()];
  expect(Math.abs((p?.x ?? 0) - (h?.x ?? 0))).toBeLessThan(30);

  // The agent reads it over MCP and answers.
  const read = (await mcpCall("comments", { name: slug })) as {
    comments: { id: number; body: string; quote: string; selector: string }[];
  };
  expect(read.comments).toHaveLength(1);
  expect(read.comments[0]?.body).toBe("Make the title bigger");
  expect(read.comments[0]?.quote).toBe("Quarterly plan");
  expect(read.comments[0]?.selector).toBe("#t");
  await mcpCall("resolve_comment", { id: read.comments[0]?.id, reply: "Title is now 2.5rem." });

  await ownerPage.reload();
  await expect(host.getByRole("button", { name: "Comments", exact: true })).toBeVisible();
  await expect(host.getByRole("button", { name: "Comment 1" })).toHaveCount(0);
  await host.getByRole("button", { name: "Comments", exact: true }).click();
  await list.getByRole("button", { name: "Show resolved (1)" }).click();
  await expect(list.getByText("Title is now 2.5rem.")).toBeVisible();
  await list.getByRole("button", { name: "Delete" }).click();
  await list.getByRole("button", { name: "Delete?" }).click();
  await expect(list.getByText("No comments yet.", { exact: false })).toBeVisible();
});

test("the page's own layout is not shifted by the bar", async () => {
  const slug = publish("Layout demo", { body: "<body style='margin:0'><h1 id=h>top</h1></body>" });
  await ownerPage.goto(`/${slug}/`);
  await expect(ownerPage.locator("[data-glim-bar]")).toHaveCount(1);
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
    await expect(page.locator("[data-glim-bar]")).toHaveCount(0);
    const html = await (await page.request.get(`/${slug}/`)).text();
    expect(html).not.toContain("glim-bar");
  } finally {
    await visitor.close();
  }
});

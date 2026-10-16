import { type BrowserContext, expect, type Page, test } from "@playwright/test";
import { publish } from "./util";

const PASSWORD = "correct horse battery";
const EMAIL = "sam@example.com";

let owner: BrowserContext;
let page: Page;

test.beforeAll(async ({ browser }) => {
  owner = await browser.newContext({ viewport: { width: 1000, height: 760 } });
  // Open the bar's closed shadow root so locators reach inside it.
  await owner.addInitScript(() => {
    const attach = Element.prototype.attachShadow;
    Element.prototype.attachShadow = function (init) {
      return attach.call(this, { ...init, mode: "open" });
    };
  });
  page = await owner.newPage();
  await page.goto("/");
  if (await page.getByRole("heading", { name: "Create your account" }).isVisible()) {
    await page.getByLabel("Email").fill(EMAIL);
    await page.getByLabel("Password", { exact: true }).fill(PASSWORD);
    await page.getByLabel("Confirm password").fill(PASSWORD);
    await page.getByRole("button", { name: "Create account" }).click();
  } else if (await page.getByRole("heading", { name: "Sign in to glim" }).isVisible()) {
    await page.getByLabel("Email").fill(EMAIL);
    await page.getByLabel("Password").fill(PASSWORD);
    await page.getByRole("button", { name: "Sign in" }).click();
  }
  await expect(page.getByRole("heading", { name: "Sign in to glim" })).toHaveCount(0);
});

test.afterAll(async () => {
  await owner.close();
});

test("the dashboard makes and plays a preview's audio version", async () => {
  const slug = publish("Listen card", { body: "<h1>Listen card</h1><p>Hello.</p>" });
  await page.goto("/");
  const card = page.getByRole("article", { name: "Listen card" });
  await card.hover();
  await card.getByRole("button", { name: "Listen" }).click();
  const dialog = page.getByRole("dialog", { name: "Listen to Listen card" });
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Make the audio version" }).click();
  await expect(dialog.getByText("A short test page.")).toBeVisible({ timeout: 10_000 });
  const audio = dialog.locator("audio");
  await expect(audio).toBeVisible();
  await expect(audio).toHaveAttribute(
    "src",
    new RegExp(`/_glim/api/previews/${slug}/listen/audio\\?v=`),
  );
  await dialog.getByText("Transcript").click();
  const text = dialog.getByText("This page is a short test page with a heading.");
  await expect(text).toBeVisible();
  // One scroll only: the dialog's, not a second one inside the transcript.
  expect(await text.evaluate((el) => getComputedStyle(el).overflowY)).toBe("visible");
  await expect(dialog.getByRole("button", { name: "Make it again" })).toBeVisible();
  await dialog.getByRole("button", { name: "1.5×" }).click();
  await expect(dialog.getByRole("button", { name: "1.5×" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await dialog.getByRole("button", { name: "Close" }).click();
  await expect(dialog).toBeHidden();
});

test("the glim bar makes the audio version and loads it", async () => {
  const slug = publish("Listen bar", { body: "<h1>Listen bar</h1><p>Hello.</p>" });
  await page.goto(`/${slug}/`);
  const host = page.locator("[data-glim-bar]");
  await host.getByRole("button", { name: "Listen" }).click();
  const panel = host.getByRole("group", { name: "Listen" });
  await panel.getByRole("button", { name: "Make the audio version" }).click();
  await expect(panel.getByText("A short test page.")).toBeVisible({ timeout: 10_000 });
  await expect(panel.locator("audio")).toHaveAttribute("src", /^blob:/);
  await panel.getByText("Transcript").click();
  const text = panel.getByText("This page is a short test page with a heading.");
  await expect(text).toBeVisible();
  // One scroll only: the panel's, not a second one inside the transcript.
  expect(await text.evaluate((el) => getComputedStyle(el).overflowY)).toBe("visible");
  // Reopening shows the saved version without making a new one.
  await host.getByRole("button", { name: "Listen" }).click();
  await expect(panel).toHaveCount(0);
  await host.getByRole("button", { name: "Listen" }).click();
  await expect(
    host.getByRole("group", { name: "Listen" }).getByText("A short test page."),
  ).toBeVisible();
});

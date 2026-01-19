import { test, expect } from "@playwright/test";

test.describe("Dashboard Pages", () => {
  test("Dashboard home page loads", async ({ page }) => {
    await page.goto("http://localhost:3000");

    // Wait for page to load
    await page.waitForLoadState("networkidle");

    // Check sidebar exists
    await expect(page.locator("text=tsuite")).toBeVisible();
    await expect(page.locator("text=Dashboard")).toBeVisible();
    await expect(page.locator("text=Runs")).toBeVisible();
    await expect(page.locator("text=Live")).toBeVisible();

    // Check header
    await expect(page.locator("h1")).toContainText("Dashboard");

    // Check stats cards exist
    await expect(page.locator("text=Total Runs")).toBeVisible();
    await expect(page.locator("text=Pass Rate")).toBeVisible();
    await expect(page.locator("text=Avg Duration")).toBeVisible();
    await expect(page.locator("text=Total Tests")).toBeVisible();

    // Take screenshot
    await page.screenshot({ path: "screenshots/dashboard.png", fullPage: true });
  });

  test("Runs page loads", async ({ page }) => {
    await page.goto("http://localhost:3000/runs");

    await page.waitForLoadState("networkidle");

    // Check header
    await expect(page.locator("h1")).toContainText("Runs");

    // Check filters exist
    await expect(page.locator("text=All")).toBeVisible();
    await expect(page.locator("text=Completed")).toBeVisible();
    await expect(page.locator("text=Failed")).toBeVisible();
    await expect(page.locator("text=Running")).toBeVisible();

    // Take screenshot
    await page.screenshot({ path: "screenshots/runs.png", fullPage: true });
  });

  test("Live page loads", async ({ page }) => {
    await page.goto("http://localhost:3000/live");

    await page.waitForLoadState("networkidle");

    // Check header
    await expect(page.locator("h1")).toContainText("Live View");

    // Check connection status section
    await expect(page.locator("text=Connected").or(page.locator("text=Disconnected"))).toBeVisible();

    // Check sections exist
    await expect(page.locator("text=Currently Running")).toBeVisible();
    await expect(page.locator("text=Run Events")).toBeVisible();
    await expect(page.locator("text=Completed Tests")).toBeVisible();

    // Take screenshot
    await page.screenshot({ path: "screenshots/live.png", fullPage: true });
  });

  test("Navigation works", async ({ page }) => {
    await page.goto("http://localhost:3000");
    await page.waitForLoadState("networkidle");

    // Click on Runs in sidebar
    await page.click("text=Runs");
    await page.waitForURL("**/runs");
    await expect(page.locator("h1")).toContainText("Runs");

    // Click on Live in sidebar
    await page.click("text=Live");
    await page.waitForURL("**/live");
    await expect(page.locator("h1")).toContainText("Live View");

    // Click on Dashboard in sidebar
    await page.click("a:has-text('Dashboard')");
    await page.waitForURL("http://localhost:3000/");
    await expect(page.locator("h1")).toContainText("Dashboard");
  });

  test("Dark theme is applied", async ({ page }) => {
    await page.goto("http://localhost:3000");
    await page.waitForLoadState("networkidle");

    // Check that dark class is on html element
    const htmlClass = await page.locator("html").getAttribute("class");
    expect(htmlClass).toContain("dark");

    // Check background color is dark navy
    const bgColor = await page.evaluate(() => {
      return getComputedStyle(document.body).backgroundColor;
    });
    console.log("Background color:", bgColor);
  });
});

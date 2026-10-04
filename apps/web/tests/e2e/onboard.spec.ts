import { expect, test } from "@playwright/test";

// Onboarding: validation and error states never need GitHub access.
test("onboard page validates empty repository URL", async ({ page }) => {
  await page.goto("/onboard");
  await expect(page.getByRole("heading", { name: "Add a repository" })).toBeVisible();
  await page.getByRole("button", { name: "inspect" }).click();
  await expect(page.getByText("paste a repository URL first")).toBeVisible();
});

test("onboard page rejects non-GitHub hosts", async ({ page }) => {
  await page.goto("/onboard");
  await page.getByLabel("Repository URL").fill("https://gitlab.com/o/r");
  await page.getByRole("button", { name: "inspect" }).click();
  await expect(page.getByLabel("repository onboarding").getByText(/INVALID_INPUT|only github\.com/i).first()).toBeVisible();
});

test("onboard page surfaces unknown repositories", async ({ page }) => {
  await page.goto("/onboard");
  await page.getByLabel("Repository URL").fill("https://github.com/o/does-not-exist-xyz");
  await page.getByRole("button", { name: "inspect" }).click();
  await expect(page.getByLabel("repository onboarding").getByText(/REPO_NOT_FOUND|Failed to fetch|404|GITHUB_UNAVAILABLE/).first()).toBeVisible();
});

test("projects page lists or empties honestly", async ({ page }) => {
  await page.goto("/projects");
  await expect(page.getByRole("heading", { name: "Projects" })).toBeVisible();
  // Always present (creation entry); rows appear only when projects exist.
  await expect(page.getByText("onboard a repository")).toBeVisible();
});

test("unknown project shows empty state", async ({ page }) => {
  await page.goto("/projects/prj_does_not_exist");
  await expect(page.getByText("Not found.")).toBeVisible();
});

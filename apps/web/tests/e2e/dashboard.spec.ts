import { expect, test } from "@playwright/test";

// Dashboard overview: loads against the live API (or shows the API error
// state when the backend is down). Both are asserted — never silent.
test("overview loads with summary and sections", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Quorum" })).toBeVisible();
  await expect(page.getByLabel("summary")).toBeVisible();
  for (const name of ["Releases", "Builders", "Policies", "Audit trail"]) {
    await expect(page.getByRole("heading", { name }).first()).toBeVisible();
  }
});

test("nav reaches every section page", async ({ page }) => {
  for (const [href, title] of [["/releases", "Releases"], ["/builders", "Builders"], ["/policies", "Policies"], ["/audit", "Audit trail"], ["/evidence", "Evidence explorer"], ["/blockchain", "Blockchain explorer"]] as const) {
    await page.goto(href);
    await expect(page.getByRole("heading", { name: title })).toBeVisible();
  }
});

test("verification lookup validates empty input", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: "inspect" }).click();
  await expect(page.getByText("enter a verification id")).toBeVisible();
});

test("verification lookup surfaces unknown id", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("verification id").fill("does-not-exist");
  await page.getByRole("button", { name: "inspect" }).click();
  // API 404 (VERIFICATION_NOT_FOUND) or unreachable backend: either way an
  // inline lookup error appears — never a silent empty panel.
  await expect(page.getByLabel("verification lookup").getByText(/does-not-exist|Failed to fetch|404/).first()).toBeVisible();
});

test("evidence explorer validates empty input", async ({ page }) => {
  await page.goto("/evidence");
  await page.getByRole("button", { name: "evidence" }).click();
  await expect(page.getByText("enter an evidence or attestation id")).toBeVisible();
});

test("unknown verification detail shows empty state", async ({ page }) => {
  await page.goto("/verifications/does-not-exist");
  await expect(page.getByText("Not found.")).toBeVisible();
});

test("blockchain explorer validates empty input", async ({ page }) => {
  await page.goto("/blockchain");
  await expect(page.getByRole("heading", { name: "Blockchain explorer" })).toBeVisible();
  await expect(page.getByText("Local Anvil transactions")).toBeVisible();
  await page.getByRole("button", { name: "re-read anchor" }).click();
  await expect(page.getByText("contract address and verification id are required")).toBeVisible();
});

test("blockchain explorer rejects malformed address", async ({ page }) => {
  await page.goto("/blockchain");
  await page.getByLabel("contract address").fill("not-an-address");
  await page.getByLabel("verification id").fill("0x" + "1".repeat(64));
  await page.getByRole("button", { name: "re-read anchor" }).click();
  await expect(page.getByLabel("anchor lookup").getByText(/400|INVALID_INPUT|error/i).first()).toBeVisible();
});

test("basic landmarks exist for assistive tech", async ({ page }) => {
  await page.goto("/");
  await expect(page.locator("main")).toBeVisible();
  await expect(page.locator('nav[aria-label="Sections"]')).toBeVisible();
});

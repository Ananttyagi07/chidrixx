import { defineConfig } from "@playwright/test";
import { BASE_URL } from "./e2e/env";

// A real end-to-end suite against a real controlplane binary (globalSetup
// builds and starts it, globalTeardown kills it) -- deliberately exercises
// the self-hosted cookie/CLI auth path (create-tenant/create-user), not
// the Supabase-backed one, since that path needs no external account or
// secrets to run hermetically in CI. Supabase auth itself is proven
// separately, by hand, against the real project (see PROJECT_STATUS.md).
export default defineConfig({
  testDir: "./e2e/tests",
  timeout: 30_000,
  fullyParallel: false, // all tests share one real server + one real DB
  workers: 1,
  reporter: [["list"]],
  globalSetup: "./e2e/globalSetup.ts",
  globalTeardown: "./e2e/globalTeardown.ts",
  use: {
    baseURL: BASE_URL,
    // System Chrome, not Playwright's own bundled Chromium -- consistent
    // with every other live verification pass this project has used,
    // and avoids needing `playwright install --with-deps` (root/sudo)
    // in environments that don't have it.
    launchOptions: {
      executablePath: "/usr/bin/google-chrome",
      args: ["--no-sandbox", "--disable-gpu"],
    },
    screenshot: "only-on-failure",
  },
});

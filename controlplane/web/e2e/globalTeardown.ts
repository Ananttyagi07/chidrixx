import { existsSync, readFileSync, rmSync } from "node:fs";
import { E2E_DIR, PID_FILE } from "./env";

export default async function globalTeardown() {
  if (existsSync(PID_FILE)) {
    const pid = Number(readFileSync(PID_FILE, "utf-8").trim());
    try {
      process.kill(pid, "SIGTERM");
    } catch {
      // already gone
    }
  }
  rmSync(E2E_DIR, { recursive: true, force: true });
}

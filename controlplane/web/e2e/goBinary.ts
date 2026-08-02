import { existsSync } from "node:fs";
import { execFileSync } from "node:child_process";

// The real Go toolchain isn't always on PATH in every dev shell (it
// wasn't in this one, needing /usr/local/go/bin explicitly) -- CI
// (GitHub Actions' setup-go) puts it on PATH properly, so this only
// exists to help local runs, not to paper over a genuinely missing Go
// install (the final fallback still fails loudly, not silently).
export function findGo(): string {
  try {
    execFileSync("go", ["version"], { stdio: "ignore" });
    return "go";
  } catch {
    // not on PATH, fall through to known install locations
  }

  for (const candidate of ["/usr/local/go/bin/go", "/usr/lib/go/bin/go"]) {
    if (existsSync(candidate)) return candidate;
  }

  throw new Error("go toolchain not found on PATH or in known install locations — install Go or add it to PATH");
}

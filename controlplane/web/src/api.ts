import type { DashboardSummary } from "./types";

export async function fetchDashboardSummary(): Promise<DashboardSummary> {
  const res = await fetch("/api/v1/dashboard-summary");

  if (!res.ok) {
    throw new Error(`dashboard-summary: HTTP ${res.status}`);
  }

  return res.json();
}

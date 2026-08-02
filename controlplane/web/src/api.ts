import type { DashboardSummary } from "./types";
import { apiFetch } from "./apiFetch";

export async function fetchDashboardSummary(): Promise<DashboardSummary> {
  const res = await apiFetch("/api/v1/dashboard-summary");

  if (!res.ok) {
    throw new Error(`dashboard-summary: HTTP ${res.status}`);
  }

  return res.json();
}

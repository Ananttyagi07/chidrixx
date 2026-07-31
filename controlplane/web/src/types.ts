// Mirrors controlplane/summary_api.go's dashboardSummaryResponse and the
// underlying store.go structs exactly — no field here is invented on the
// frontend side.

export interface Finding {
  source: string;
  destination: string;
  path_class: string;
  confidence: string;
  bytes_tx: number;
  bytes_rx: number;
  cost_low_inr: number;
  cost_high_inr: number;
  fix_hint: string;
  fix_manifest: string;
}

export interface FindingRow extends Finding {
  ClusterID: string;
  ReportedAt: string;
}

export interface Summary {
  ClusterCount: number;
  WorkloadCount: number;
  FindingCount: number;
  TotalBytesTx: number;
  TotalBytesRx: number;
  TotalCostLowINR: number;
  TotalCostHighINR: number;
}

export interface ClassSpend {
  PathClass: string;
  CostHighINR: number;
  FindingCount: number;
}

export interface CostTrendPoint {
  ReportedAt: string;
  CostHigh: number;
}

export interface ClusterSummaryView {
  ClusterID: string;
  LastSeen: string;
  FindingCount: number;
  CostLowINR: number;
  CostHighINR: number;
  trend: CostTrendPoint[];
}

export interface DashboardSummary {
  summary: Summary;
  spend_by_class: ClassSpend[];
  trend: CostTrendPoint[];
  clusters: ClusterSummaryView[];
  top_fixes: FindingRow[];
}

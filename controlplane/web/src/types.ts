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
  cloud: string;
  region: string;
  savings_low_inr: number;
  savings_high_inr: number;
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

export interface CloudSpend {
  Cloud: string;
  Region: string;
  CostHighINR: number;
  FindingCount: number;
}

export interface ClusterSummaryView {
  ClusterID: string;
  LastSeen: string;
  FindingCount: number;
  CostLowINR: number;
  CostHighINR: number;
  Cloud: string;
  Region: string;
  trend: CostTrendPoint[];
}

export interface DeployEvent {
  namespace: string;
  name: string;
  reason: string;
  message: string;
  occurred_at: string;
}

export interface AnomalyPoint {
  cluster_id: string;
  previous_cost_inr: number;
  current_cost_inr: number;
  growth_ratio: number;
  likely_cause?: DeployEvent;
}

export interface TeamSpend {
  team: string;
  cost_high_inr: number;
  finding_count: number;
}

export interface TeamOwnership {
  namespace: string;
  team: string;
}

export interface DashboardSummary {
  summary: Summary;
  spend_by_class: ClassSpend[];
  spend_by_cloud: CloudSpend[];
  spend_by_team: TeamSpend[];
  trend: CostTrendPoint[];
  clusters: ClusterSummaryView[];
  top_fixes: FindingRow[];
  anomalies: AnomalyPoint[];
}

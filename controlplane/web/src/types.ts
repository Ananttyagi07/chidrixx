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

export interface DeepForecastPoint {
  h: number;
  forecast: number;
  lower: number;
  upper: number;
}

export interface DeepForecastResult {
  model: "holt" | "damped_holt";
  alpha: number;
  beta: number;
  phi: number;
  points_retained: number;
  points_used_for_fit: number;
  backtest_folds: number;
  backtest_mae_holt: number;
  backtest_mae_damped: number;
  forecast: DeepForecastPoint[];
}

export interface ForecastResponse {
  available: boolean;
  result?: DeepForecastResult;
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

export interface Invite {
  email: string;
  role: "admin" | "viewer";
  created_at: string;
}

export interface WorkloadCostPoint {
  reported_at: string;
  cost_high: number;
}

export interface WorkloadGrowth {
  cluster_id: string;
  workload: string;
  trend: WorkloadCostPoint[];
  delta_inr: number;
  related_events?: DeployEvent[];
}

export interface RemediationDecision {
  cluster_id: string;
  source: string;
  destination: string;
  path_class: string;
  confidence: string;
  fix_hint: string;
  fix_manifest?: string;
  cost_high_inr: number;
  savings_high_inr: number;
  would_auto_apply: boolean;
  reasons: string[];
}

export interface RemediationPreviewResponse {
  decisions: RemediationDecision[];
}

export interface OutcomeDatasetStats {
  total_shown: number;
  total_applied: number;
  total_measured: number;
  mean_abs_prediction_error_inr?: number;
}

export interface AIEvalFeatureStats {
  feature: string;
  total_requests: number;
  success_count: number;
  success_rate: number;
  hit_round_limit_count: number;
  avg_latency_ms: number;
  total_tool_calls: number;
  total_tool_call_errors: number;
  tool_success_rate?: number;
  total_prompt_tokens: number;
  total_completion_tokens: number;
}

export interface AIEvalStatsResponse {
  features: AIEvalFeatureStats[];
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

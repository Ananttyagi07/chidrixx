"""Bottom-up COGS model for chidrixx, built from REAL measurements taken
from the live k3d deployment on 2026-08-05. Every input marked MEASURED
is a real number observed on the running system; every input marked
ASSUMED is a modelling choice that must be revisited with real data.
"""

# ---------------------------------------------------------------- MEASURED
ROWS_TOTAL          = 4_125_883      # MEASURED: flow_aggregate rows
DB_BYTES            = 1_195_917_312  # MEASURED: controlplane.db on disk
FINDINGS_PER_SNAP   = 123            # MEASURED: chidrixx-lab findings/snapshot
CADENCE_S           = 15             # MEASURED: real ingest cadence
CP_CPU_MILLI        = 1              # MEASURED: controlplane idle CPU (millicores)
CP_MEM_MI           = 13             # MEASURED: controlplane memory
AGENT_CPU_MILLI     = 24             # MEASURED: agent CPU per node
AGENT_MEM_MI        = 15             # MEASURED: agent memory per node
CHAT_PROMPT_TOK     = 3752           # MEASURED: real Groq prompt tokens, 1 chat turn
CHAT_COMPLETION_TOK = 114            # MEASURED: real Groq completion tokens
RETENTION_DAYS      = 30             # MEASURED: shipped default (CHIDRIXX_RAW_RETENTION_DAYS)
COMPACTION_RATIO    = 1103           # MEASURED: real 2,205,218 raw -> 1,998 rollups

# --------------------------------------------------------- MARKET (looked up)
GROQ_IN_PER_MTOK    = 0.59   # $/1M input tokens, Llama 3.3 70B on-demand
GROQ_OUT_PER_MTOK   = 0.79   # $/1M output tokens
EBS_GP3_PER_GB_MO   = 0.08   # $/GB-month (us-east-1 list; ap-south-1 runs higher)
AP_SOUTH_UPLIFT     = 1.25   # ap-south-1 ~20-30% above us-east-1
VCPU_HR             = 0.0416 / 2   # t3.medium $0.0416/hr for 2 vCPU -> per vCPU-hr
USD_INR             = 84.0   # same rate the product's own price book uses

HOURS_MO = 730

# ------------------------------------------------------------------ DERIVED
bytes_per_row = DB_BYTES / ROWS_TOTAL
snaps_per_day = 86400 / CADENCE_S
rows_per_day  = FINDINGS_PER_SNAP * snaps_per_day
bytes_per_day = rows_per_day * bytes_per_row

# Steady state: 30 days of raw rows, plus compacted rollups of everything
# older. Rollups are negligible (1103x reduction) but counted honestly.
raw_steady_gb = bytes_per_day * RETENTION_DAYS / 1e9
# A year of history beyond the window, folded into daily rollups:
rollup_steady_gb = (bytes_per_day * 335 / COMPACTION_RATIO) / 1e9
storage_gb_per_cluster = raw_steady_gb + rollup_steady_gb

storage_usd_mo = storage_gb_per_cluster * EBS_GP3_PER_GB_MO * AP_SOUTH_UPLIFT

# Compute: the control plane is essentially idle at this scale, but a real
# hosted tenant needs headroom for dashboard queries. Measured idle is 1m
# CPU; the real observed load spike during dashboard-summary was ~1 full
# core for a few seconds. Model a conservative sustained average.
cp_cpu_cores_per_cluster = 0.05   # ASSUMED: 50 millicores sustained per cluster
compute_usd_mo = cp_cpu_cores_per_cluster * VCPU_HR * HOURS_MO * AP_SOUTH_UPLIFT

# AI: real measured tokens per chat turn.
chat_usd = (CHAT_PROMPT_TOK / 1e6) * GROQ_IN_PER_MTOK + \
           (CHAT_COMPLETION_TOK / 1e6) * GROQ_OUT_PER_MTOK

print("=" * 68)
print("REAL MEASURED UNIT ECONOMICS  (chidrixx, 2026-08-05)")
print("=" * 68)
print(f"bytes per stored row              : {bytes_per_row:,.0f} B   (MEASURED)")
print(f"snapshots per day @ {CADENCE_S}s        : {snaps_per_day:,.0f}")
print(f"rows/day for a {FINDINGS_PER_SNAP}-flow cluster : {rows_per_day:,.0f}")
print(f"raw bytes/day per cluster         : {bytes_per_day/1e6:,.1f} MB")
print()
print(f"steady-state storage / cluster    : {storage_gb_per_cluster:,.2f} GB")
print(f"  = {raw_steady_gb:.2f} GB raw ({RETENTION_DAYS}d) + {rollup_steady_gb:.3f} GB rollups (1yr)")
print()
print("--- monthly COGS per ACTIVE CLUSTER (this lab's real profile) ---")
print(f"  storage   : ${storage_usd_mo:6.3f}")
print(f"  compute   : ${compute_usd_mo:6.3f}")
print(f"  subtotal  : ${storage_usd_mo + compute_usd_mo:6.3f}  (Rs {(storage_usd_mo+compute_usd_mo)*USD_INR:,.1f})")
print()
print("--- AI feature, per real request ---")
print(f"  1 chat turn ({CHAT_PROMPT_TOK} in + {CHAT_COMPLETION_TOK} out) : ${chat_usd:.5f}  (Rs {chat_usd*USD_INR:.3f})")
for n in (100, 1000, 5000):
    print(f"  {n:>5} chat turns/mo                 : ${chat_usd*n:7.2f}")
print()

# ---- Scale sensitivity: the lab cluster is TINY. Real clusters are bigger.
print("=" * 68)
print("SCALE SENSITIVITY  (storage COGS is linear in flows x cadence)")
print("=" * 68)
print(f"{'flows/snap':>11} {'cadence':>8} {'GB steady':>11} {'$/mo':>8} {'Rs/mo':>9}")
for flows, cad in [(123, 15), (500, 15), (2000, 15), (2000, 60), (10000, 60), (10000, 300)]:
    rpd = flows * (86400 / cad)
    bpd = rpd * bytes_per_row
    gb = (bpd * RETENTION_DAYS + bpd * 335 / COMPACTION_RATIO) / 1e9
    usd = gb * EBS_GP3_PER_GB_MO * AP_SOUTH_UPLIFT
    print(f"{flows:>11} {str(cad)+'s':>8} {gb:>11,.1f} {usd:>8,.2f} {usd*USD_INR:>9,.0f}")
print()
print("NOTE: at 15s cadence the storage cost is dominated by CADENCE, not")
print("flow count. Raising the default ingest interval is the single")
print("biggest COGS lever available and costs nothing to implement.")

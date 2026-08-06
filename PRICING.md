# chidrixx — Pricing, Unit Economics & Market Position

_Built 2026-08-05. Same discipline as PROJECT_STATUS.md: every number is
tagged **MEASURED** (observed on the real running system), **LOOKED UP**
(a real external price, with the date it was checked), or **ASSUMED** (a
modelling choice that is not yet backed by evidence and must be revisited
with real customer data). Nothing here is a guess presented as a fact._

_Every figure in this document is reproducible: `python3 pricing/costmodel.py`
(unit economics), `pricing/tiers.py` (per-tier COGS and margin),
`pricing/retention.py` (the retention/cadence sensitivity in §4). The
measured inputs are constants at the top of `costmodel.py` — re-measure
and re-run rather than hand-editing any number below._

---

## 0. The headline finding, up front

The method requested was: measure infra cost per feature → add margin →
check it lands inside the market band. That was done, in full, below.

**It produced an answer that says the method shouldn't set the price.**

Real measured cost to serve one active cluster is **$1.57/month**
(₹132). The market anchor for this category is **$8/node/month**. A
cost-plus price — even at a fat 300% markup — would come out around
**$5/month per customer**, which is roughly 1/100th of what the market
already pays for less. It would leave almost all the value on the table
*and* signal that the product is cheap.

So the bottom-up work below is not wasted, but its real job is different
from what it was commissioned for. It answers three questions that
actually matter:

1. **What's the floor?** (What price would lose money — so no plan is
   ever accidentally unprofitable.)
2. **Which feature is actually expensive?** (Answer: not the AI. Storage,
   by a wide margin — 78–94% of all COGS.)
3. **Where is the margin trap?** (Long retention on large accounts. One
   plausible plan shape below lands at **36.8% gross margin** — a real
   problem that only bottom-up modelling would have caught.)

Price should be set from **market and value**; cost should be used to set
the **floor** and to **design the plan limits**. Both are below.

---

## 1. Measured inputs (the real system, 2026-08-05)

| Input | Value | Source |
|---|---|---|
| Bytes per stored flow row | **290 B** | MEASURED — 1,195,917,312 B on disk / 4,125,883 real rows |
| Real ingest cadence | **15 s** | MEASURED — consecutive `reported_at` on the live cluster |
| Flows per snapshot (lab) | **123** | MEASURED — `chidrixx-lab` real finding count |
| Control-plane CPU / memory | **1 m / 13 Mi** | MEASURED — `kubectl top`, idle |
| Agent CPU / memory per node | **24 m / 15 Mi** | MEASURED — `kubectl top` |
| Real tokens, one chat turn | **3,752 in + 114 out** | MEASURED — real Groq response via `ai_eval_events` |
| Compaction reduction | **1,103×** | MEASURED — 2,205,218 raw rows → 1,998 rollups |
| Default raw retention | **30 days** | MEASURED — shipped `CHIDRIXX_RAW_RETENTION_DAYS` |

Derived from the above, arithmetic only:

- 5,760 snapshots/day at 15 s → **708,480 rows/day** for a 123-flow cluster
- → **205 MB/day**, → **6.22 GB** steady state at 30-day retention
  (6.16 GB raw + 0.06 GB of year-old rollups)

## 2. External prices (LOOKED UP 2026-08-05 — re-verify before committing)

| Item | Price | Note |
|---|---|---|
| Groq Llama 3.3 70B | **$0.59 / $0.79** per 1M in/out tokens | Batch API ~50% cheaper; prompt caching another ~50% |
| AWS EBS gp3 | **$0.08 / GB-month** | us-east-1 list; ap-south-1 modelled at **+25%** |
| EC2 vCPU (t3.medium basis) | **~$0.0208 / vCPU-hour** | us-east-1 list, +25% for ap-south-1 |
| **Kubecost (IBM)** | **$8 / node / month** | The category's public anchor. Free tier to 250 cores, 15-day retention |
| **Datadog CCM** | **from $8 / host / month** | Their infra monitoring alone is $15–27/host |
| **Finout** | **from $1,000 / month** | **+25% surcharge** specifically for Kubernetes |
| CloudZero, Vantage | custom / spend-based | No public per-node list price |

## 3. Cost per feature, per month

The point of this table: **the AI features people assume are expensive
are the cheapest thing here.**

| Feature | Real driver | Cost | Share |
|---|---|---|---|
| Flow ingest + storage | 290 B/row × cadence × retention | **$0.62** per lab-profile cluster | **78–94% of all COGS** |
| Dashboard / API compute | ~50 m CPU sustained per cluster | $0.95 per cluster | ~12% |
| **AI chat assistant** | 3,866 real tokens/turn | **$0.0023 per turn** | ~10% at 3,000 turns/mo |
| Anomaly narrator | single completion, no tool loop | < $0.001 per call | negligible |
| Proactive anomaly watch | existing SQL on a 5-min tick | ~0 marginal | negligible |
| Forecasting, placement sim, remediation, traffic replay | pure Go over already-fetched data | **$0 marginal** | 0% |

Three real consequences:

- **The AI is not a cost problem.** 1,000 chat turns/month costs **$2.30**.
  Even 5,000 turns costs $11.52. Bundling AI into every paid tier is
  affordable; gating it behind a premium add-on would be pricing theatre,
  not cost recovery. *(Do still rate-limit it — see §6.)*
- **Six of the shipped features have literally zero marginal cost** — they
  are pure computation over data already stored. They are pure margin and
  should be used as differentiation, not metered.
- **Storage is the whole game**, and it is driven by two numbers that are
  *our own defaults*, not customer requirements: **ingest cadence** and
  **retention window**.

## 4. The margin trap (the real find)

Storage cost = flows × (1/cadence) × retention. All three multiply. Same
product, same customer, wildly different economics:

**Business shape — 8 clusters × 2,500 flows, priced at $500/mo:**

| Cadence | Retention | Storage | Gross margin |
|---|---|---|---|
| 15 s | 90 d | $301.50 | **36.8%** ← loses the business |
| 15 s | 30 d | $101.24 | 76.9% |
| 30 s | 30 d | $50.62 | 87.0% |
| 60 s | 30 d | $25.31 | **92.0%** |

**Enterprise shape — 25 clusters × 6,000 flows:**

| Cadence | Retention | Storage | Margin @ $2,500 | Margin @ $6,000 |
|---|---|---|---|---|
| 30 s | 90 d | $1,130.64 | **51.9%** | 79.9% |
| 60 s | 90 d | $565.32 | 74.5% | 89.4% |
| 60 s | 30 d | $189.82 | 89.5% | 95.6% |

**Conclusions that follow directly from these numbers:**

1. **Ship a 30 s default cadence, not 15 s.** It halves the single largest
   cost line with no meaningful loss of fidelity for a *cost* tool (this
   is billing data, not latency tracing). This is a one-line default
   change and the highest-leverage margin decision available.
2. **Retention must be a priced dimension, never a free upgrade.** 90-day
   retention is 3× the cost of 30-day. Sell it as an add-on.
3. **Flat-fee enterprise at $2,500/mo is underpriced** if it includes long
   retention on large fleets. Kubecost's reported enterprise deals run far
   higher; $6,000/mo restores an 80%+ margin and is still credible.

## 5. Proposed plans

Priced against the **market anchor** ($8/node Kubecost), deliberately
undercutting it, with limits designed from the cost model above.

| | **Free** | **Team** | **Business** | **Enterprise** |
|---|---|---|---|---|
| **Price** | $0 | **$5 / node / mo** | **$5 / node / mo** (100+ nodes) | **from $6,000 / mo** flat |
| Clusters | 1 | 5 | 25 | unlimited |
| Nodes | up to 10 | up to 100 | 100+ | unlimited |
| Retention | 7 days | 30 days | 30 days | 90 days included |
| Ingest cadence | 60 s | 30 s | 30 s | 30 s (tunable) |
| Path classification + fix engine | ✅ | ✅ | ✅ | ✅ |
| Multi-cluster, teams, budgets | 1 cluster | ✅ | ✅ | ✅ |
| Forecasting, cost graph, placement sim | ✅ | ✅ | ✅ | ✅ |
| AI assistant + anomaly narrator | 50 turns/mo | 500/mo | 3,000/mo | 15,000/mo |
| Proactive anomaly watch | — | ✅ | ✅ | ✅ |
| Remediation preview + traffic-replay safety | — | ✅ | ✅ | ✅ |
| SSO / self-hosted / support SLA | — | — | — | ✅ |
| **Real COGS at that shape** | **$0.56** | **$10.07** | **$65.12** | **~$1,204** |
| **Gross margin** | n/a | **89.9%** | **87.0%** | **~80%** |

Every tier clears the 70–80% SaaS gross-margin bar, and the free tier
costs **$0.56/month** to serve — cheap enough that generous free usage is
a viable acquisition strategy rather than a liability.

**Why $5/node:** it sits deliberately under Kubecost's $8 public anchor, so
"cheaper than the incumbent *and* you get eBPF-accurate attribution they
don't have" is a single clean sentence. At $5/node a 100-node account
bills $500/mo against ~$65 of real cost.

## 6. Guardrails the cost model says to build

- **Rate-limit the AI per plan.** Headroom is large (Team tolerates ~39,000
  extra chat turns before margin hits zero) but it is the one cost that
  scales with *user behaviour* rather than plan shape. Cap it and show
  usage in-product; don't discover abuse on the invoice.
- **Enforce cadence and retention server-side**, per plan. They are the
  entire margin story; if a customer can set 5 s cadence on a free tier,
  the model breaks.
- **Bill on nodes, not on flows or GB.** Nodes are what the market already
  understands and what buyers can predict. Our costs scale with flows, so
  keep an internal alert if any account's flows-per-node runs far above
  model — that's the leading indicator of a bad-fit account, and it's
  measurable today via `kharcha_map_utilization_ratio` (§5.3 of
  PROJECT_STATUS.md).

## 7. What this analysis does NOT establish

Stated plainly, because the rest of this document is grounded and this
part isn't:

- **Willingness to pay is unmeasured.** No customer has been asked. Every
  price above is anchored to competitors' published prices, which is a
  reasonable starting point and *not* evidence anyone will pay it.
- **The tier shapes are ASSUMED.** Cluster counts, flows-per-snapshot and
  chat volumes per plan are modelling choices. The lab cluster runs 123
  flows/snapshot; a real production cluster's number is unknown and
  changes COGS linearly.
- **Storage costs assume self-hosting on raw EBS.** A managed database,
  replication, or backups would add real cost not modelled here.
- **No CAC, churn, or sales cost.** This is gross margin, not net. A
  product with 89% gross margin can still lose money on go-to-market.
- **Compaction beyond the retention window is modelled at the real
  measured 1,103× ratio**, but has never actually run on live data (the
  live cluster's history is shorter than the 30-day window) — verified as
  correct against a real 2.37M-row copy, not in live steady state.

The single highest-value next step is not more modelling. It is putting
these three prices in front of five real Kubernetes operators and asking
which one they'd pay — that replaces the one genuinely unfounded
assumption above with evidence.

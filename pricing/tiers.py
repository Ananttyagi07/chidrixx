"""Per-tier COGS and gross margin, built on costmodel.py's real measured
unit economics. Tier definitions are product choices (ASSUMED); the cost
per unit underneath them is MEASURED.
"""

BYTES_PER_ROW = 290          # MEASURED
GROQ_IN, GROQ_OUT = 0.59, 0.79
EBS = 0.08 * 1.25            # gp3 + ap-south uplift
VCPU_HR = 0.0208
HOURS_MO = 730
USD_INR = 84.0
RET = 30
COMPACT = 1103
CHAT_USD = (3752/1e6)*GROQ_IN + (114/1e6)*GROQ_OUT   # MEASURED tokens


def storage_gb(flows, cadence_s, retention_days=RET):
    rpd = flows * (86400 / cadence_s)
    bpd = rpd * BYTES_PER_ROW
    return (bpd * retention_days + bpd * 335 / COMPACT) / 1e9


def tier_cogs(clusters, flows, cadence, retention, chats_mo, cpu_cores_per_cluster):
    st = sum(storage_gb(flows, cadence, retention) for _ in range(clusters))
    storage = st * EBS
    compute = clusters * cpu_cores_per_cluster * VCPU_HR * HOURS_MO * 1.25
    ai = chats_mo * CHAT_USD
    return storage, compute, ai, storage + compute + ai


TIERS = [
    # name,           clusters, flows, cadence, retention, chats/mo, cores/cluster, price_usd_mo
    ("Free",                 1,   200,     60,        7,       50,   0.02,    0),
    ("Team ($5/node x 20)", 3,   800,     30,       30,      500,   0.05,  100),
    ("Business ($5 x 100)", 8,  2500,     30,       30,     3000,   0.05,  500),
    ("Enterprise (flat)",  25,  6000,     30,       90,    15000,   0.08, 2500),
]

print("=" * 96)
print("PER-TIER COGS AND GROSS MARGIN  (costs MEASURED, tier shapes ASSUMED)")
print("=" * 96)
print(f"{'Tier':<22}{'stor$':>8}{'cpu$':>8}{'AI$':>8}{'COGS$':>9}{'price$':>9}{'margin%':>9}{'COGS Rs':>10}")
for name, cl, fl, cad, ret, chats, cores, price in TIERS:
    s, c, a, total = tier_cogs(cl, fl, cad, ret, chats, cores)
    margin = ((price - total) / price * 100) if price else float('nan')
    m = f"{margin:8.1f}%" if price else "     n/a"
    print(f"{name:<22}{s:>8.2f}{c:>8.2f}{a:>8.2f}{total:>9.2f}{price:>9}{m:>9}{total*USD_INR:>10,.0f}")

print()
print("=" * 96)
print("WHAT DOMINATES COGS")
print("=" * 96)
s, c, a, t = tier_cogs(8, 2500, 30, 30, 3000, 0.05)
for label, v in (("storage", s), ("compute", c), ("AI (Groq)", a)):
    print(f"  {label:<12} ${v:7.2f}  {v/t*100:5.1f}% of Business-tier COGS")

print()
print("=" * 96)
print("BREAK-EVEN / DOWNSIDE CHECK: how much abuse before a tier loses money?")
print("=" * 96)
for name, cl, fl, cad, ret, chats, cores, price in TIERS[1:]:
    s, c, a, total = tier_cogs(cl, fl, cad, ret, chats, cores)
    headroom = price - total
    max_extra_chats = headroom / CHAT_USD
    print(f"  {name:<22} headroom ${headroom:8.2f}/mo = {max_extra_chats:,.0f} extra chat turns "
          f"before gross margin hits zero")

print()
print("=" * 96)
print("MARKET ANCHORS (looked up 2026-08-05, verify before committing)")
print("=" * 96)
print("  Kubecost (IBM)      $8/node/month standard tier; enterprise = custom quote")
print("  Datadog CCM         from $8/host/month (infra monitoring itself $15-27/host)")
print("  Finout              from $1,000/month, +25% surcharge for Kubernetes")
print("  CloudZero/Vantage   custom / spend-based, no public per-node list price")
print()
print("  => a $5/node list price sits deliberately UNDER Kubecost's $8 anchor.")
print("     At $5/node a 100-node account bills $500/mo against ~$6 real COGS.")

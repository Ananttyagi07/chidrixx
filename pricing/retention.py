"""Retention is the dominant COGS lever at enterprise scale. This isolates
it so the pricing decision is made on real numbers, not instinct.
"""
BYTES_PER_ROW = 290
EBS = 0.08 * 1.25
COMPACT = 1103
USD_INR = 84.0


def storage_gb(flows, cadence_s, retention_days, history_days=365):
    rpd = flows * (86400 / cadence_s)
    bpd = rpd * BYTES_PER_ROW
    older = max(history_days - retention_days, 0)
    return (bpd * retention_days + bpd * older / COMPACT) / 1e9


print("=" * 88)
print("ENTERPRISE SHAPE: 25 clusters x 6000 flows/snapshot")
print("=" * 88)
print(f"{'cadence':>9}{'retention':>11}{'GB':>12}{'storage $/mo':>15}{'margin@$2500':>14}{'margin@$6000':>14}")
for cad in (30, 60, 300):
    for ret in (7, 30, 90):
        gb = 25 * storage_gb(6000, cad, ret)
        usd = gb * EBS
        cogs = usd + 37.96 + 34.56          # + measured compute + AI from tiers.py
        m25 = (2500 - cogs) / 2500 * 100
        m60 = (6000 - cogs) / 6000 * 100
        print(f"{str(cad)+'s':>9}{str(ret)+'d':>11}{gb:>12,.0f}{usd:>15,.2f}{m25:>13.1f}%{m60:>13.1f}%")

print()
print("=" * 88)
print("THE SAME LEVER ON THE DEFAULT (Business) SHAPE: 8 clusters x 2500 flows")
print("=" * 88)
print(f"{'cadence':>9}{'retention':>11}{'GB':>12}{'storage $/mo':>15}{'margin@$500':>13}")
for cad in (15, 30, 60):
    for ret in (7, 30, 90):
        gb = 8 * storage_gb(2500, cad, ret)
        usd = gb * EBS
        cogs = usd + 7.59 + 6.91
        m = (500 - cogs) / 500 * 100
        print(f"{str(cad)+'s':>9}{str(ret)+'d':>11}{gb:>12,.0f}{usd:>15,.2f}{m:>12.1f}%")

print()
print("=" * 88)
print("READ-OUT")
print("=" * 88)
print("* Storage is ~78% of COGS at Business scale and ~94% at Enterprise scale.")
print("* Cadence and retention are multiplicative and both are OUR defaults,")
print("  not customer demands -- they are the two cheapest margin levers there are.")
print("* At 30s/90d, a 25-cluster enterprise account costs ~$1.1k/mo to serve.")
print("  Flat $2,500 gives ~52% margin: too thin for SaaS (target 70-80%+).")
print("  Either price enterprise at $6k+/mo (in line with Kubecost's reported")
print("  $70-100k/yr enterprise deals) or make 90-day retention a paid add-on.")

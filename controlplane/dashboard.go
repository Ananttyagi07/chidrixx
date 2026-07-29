package main

import (
	"html/template"
	"log"
	"net/http"
	"time"
)

const dashboardTemplateSrc = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta http-equiv="refresh" content="15">
<title>Chidrixx Control Plane</title>
<style>
  :root {
    --ink: #EEF1F5; --ink-raised: #FFFFFF; --ink-border: #DCE1E8;
    --text: #161B22; --muted: #5B6472;
    --rupee: #B8842A; --wire: #1F8695; --good: #2F8F5B; --costly: #C24E2E;
    --font-mono: ui-monospace, "SF Mono", "Cascadia Code", "Roboto Mono", "JetBrains Mono", Menlo, monospace;
    --font-sans: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  }
  @media (prefers-color-scheme: dark) {
    :root {
      --ink: #10141B; --ink-raised: #181E28; --ink-border: #262E3B;
      --text: #E7EAF0; --muted: #8993A6;
      --rupee: #D9A441; --wire: #4FB6C4; --good: #4CAF7D; --costly: #E0704F;
    }
  }
  * { box-sizing: border-box; }
  body { margin: 0; background: var(--ink); color: var(--text); font-family: var(--font-sans); line-height: 1.5; }
  .page { max-width: 72rem; margin: 0 auto; padding: 2.5rem 1.5rem 4rem; display: flex; flex-direction: column; gap: 1.75rem; }
  .eyebrow { font-family: var(--font-mono); font-size: 0.72rem; letter-spacing: 0.14em; text-transform: uppercase; color: var(--wire); }
  h1 { font-family: var(--font-mono); font-size: clamp(1.4rem, 2.6vw, 1.9rem); font-weight: 600; margin: 0.2rem 0 0; letter-spacing: -0.01em; }
  .meta { color: var(--muted); font-size: 0.88rem; }
  .meta strong { color: var(--text); font-variant-numeric: tabular-nums; }

  .hero-row { display: grid; grid-template-columns: repeat(auto-fit, minmax(11rem, 1fr)); gap: 0.85rem; }
  .hero-card { background: var(--ink-raised); border: 1px solid var(--ink-border); border-left: 3px solid var(--rupee); border-radius: 0.5rem; padding: 0.9rem 1.1rem; }
  .hero-label { font-family: var(--font-mono); font-size: 0.72rem; letter-spacing: 0.06em; color: var(--muted); }
  .hero-value { font-size: 1.7rem; font-weight: 600; font-variant-numeric: tabular-nums; margin-top: 0.15rem; }

  .section-title { font-family: var(--font-mono); font-size: 0.78rem; letter-spacing: 0.08em; text-transform: uppercase; color: var(--muted); margin: 0; }

  .cluster-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(15rem, 1fr)); gap: 0.85rem; }
  .cluster-card { background: var(--ink-raised); border: 1px solid var(--ink-border); border-radius: 0.6rem; padding: 1rem 1.1rem; display: flex; flex-direction: column; gap: 0.5rem; }
  .cluster-card-head { display: flex; align-items: baseline; justify-content: space-between; gap: 0.5rem; }
  .cluster-id { font-family: var(--font-mono); font-weight: 600; font-size: 0.95rem; }
  .cluster-seen { font-size: 0.74rem; color: var(--muted); white-space: nowrap; }
  .cluster-cost { font-size: 1.15rem; font-weight: 600; color: var(--rupee); font-variant-numeric: tabular-nums; }
  .cluster-sub { font-size: 0.76rem; color: var(--muted); }
  .cluster-spark { align-self: stretch; }

  .spark { width: 100%; height: 32px; display: block; }
  .spark-line { stroke: var(--muted); }
  .spark-area { fill: var(--muted); opacity: 0.15; }
  .spark-dot { fill: var(--rupee); stroke: var(--ink-raised); }
  .spark-empty { background: repeating-linear-gradient(90deg, var(--ink-border) 0 4px, transparent 4px 8px); border-radius: 2px; }

  .table-wrap { background: var(--ink-raised); border: 1px solid var(--ink-border); border-radius: 0.6rem; overflow: hidden; }
  .table-scroll { overflow-x: auto; }
  table { border-collapse: collapse; width: 100%; font-size: 0.83rem; min-width: 50rem; }
  th { text-align: left; font-family: var(--font-mono); font-size: 0.7rem; letter-spacing: 0.06em; text-transform: uppercase; color: var(--muted); font-weight: 500; padding: 0.7rem 0.9rem; border-bottom: 1px solid var(--ink-border); }
  td { padding: 0.55rem 0.9rem; border-bottom: 1px solid var(--ink-border); vertical-align: top; }
  tbody tr:last-child td { border-bottom: none; }
  tbody tr:hover { background: color-mix(in srgb, var(--wire) 6%, transparent); }
  .mono { font-family: var(--font-mono); font-size: 0.8rem; }
  .truncate { max-width: 14rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .num { text-align: right; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .cost { color: var(--rupee); font-weight: 600; }
  .chip { font-family: var(--font-mono); font-size: 0.7rem; padding: 0.15rem 0.5rem; border-radius: 999px; white-space: nowrap; border: 1px solid transparent; }
  .chip-costly { color: var(--costly); background: color-mix(in srgb, var(--costly) 14%, transparent); border-color: color-mix(in srgb, var(--costly) 30%, transparent); }
  .chip-wire   { color: var(--wire);   background: color-mix(in srgb, var(--wire) 14%, transparent);   border-color: color-mix(in srgb, var(--wire) 30%, transparent); }
  .chip-good   { color: var(--good);   background: color-mix(in srgb, var(--good) 14%, transparent);   border-color: color-mix(in srgb, var(--good) 30%, transparent); }
  .empty { color: var(--muted); font-size: 0.9rem; padding: 1.5rem; }
</style>
</head>
<body>
  <div class="page">
    <div>
      <div class="eyebrow">chidrixx &middot; control plane</div>
      <h1>Multi-Cluster Network Cost</h1>
      <div class="meta">{{.ClusterCount}} cluster{{if ne .ClusterCount 1}}s{{end}} reporting &middot; refreshes every 15s</div>
    </div>

    <div class="hero-row">
      <div class="hero-card">
        <div class="hero-label">TOTAL ESTIMATED COST</div>
        <div class="hero-value">&#8377;{{printf "%.4f" .TotalLow}}&ndash;{{printf "%.4f" .TotalHigh}}</div>
      </div>
      <div class="hero-card" style="border-left-color: var(--wire)">
        <div class="hero-label">CLUSTERS</div>
        <div class="hero-value">{{.ClusterCount}}</div>
      </div>
      <div class="hero-card" style="border-left-color: var(--good)">
        <div class="hero-label">FINDINGS (LATEST SNAPSHOTS)</div>
        <div class="hero-value">{{.FindingCount}}</div>
      </div>
    </div>

    <div>
      <p class="section-title">Clusters</p>
      {{if not .Clusters}}
        <div class="empty">No agents have shipped yet. Point one at this control plane's /api/v1/ingest.</div>
      {{else}}
      <div class="cluster-grid">
        {{range .Clusters}}
        <div class="cluster-card">
          <div class="cluster-card-head">
            <span class="cluster-id">{{.ClusterID}}</span>
            <span class="cluster-seen">{{.LastSeenAgo}}</span>
          </div>
          <div class="cluster-cost">&#8377;{{printf "%.4f" .CostHighINR}}</div>
          <div class="cluster-sub">{{.FindingCount}} finding{{if ne .FindingCount 1}}s{{end}}</div>
          <div class="cluster-spark">{{.Sparkline}}</div>
        </div>
        {{end}}
      </div>
      {{end}}
    </div>

    <div>
      <p class="section-title">Top findings across all clusters</p>
      <div class="table-wrap">
        <div class="table-scroll">
          <table>
            <thead>
              <tr>
                <th>Cluster</th>
                <th>Source</th>
                <th>Destination</th>
                <th>Class</th>
                <th class="num">Cost (INR)</th>
              </tr>
            </thead>
            <tbody>
              {{range .Findings}}
              <tr>
                <td class="mono">{{.ClusterID}}</td>
                <td class="mono truncate" title="{{.Source}}">{{.Source}}</td>
                <td class="mono truncate" title="{{.Destination}}">{{.Destination}}</td>
                <td><span class="chip chip-{{.Tone}}">{{.PathClass}}</span></td>
                <td class="num mono cost">&#8377;{{printf "%.4f" .CostLowINR}}&ndash;{{printf "%.4f" .CostHighINR}}</td>
              </tr>
              {{end}}
              {{if not .Findings}}
              <tr><td colspan="5" class="empty">No findings yet.</td></tr>
              {{end}}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </div>
</body>
</html>
`

var dashboardTemplate = template.Must(template.New("dashboard").Parse(dashboardTemplateSrc))

var classTone = map[string]string{
	"INTERNET_EGRESS":    "costly",
	"CROSS_REGION":       "costly",
	"NAT_EGRESS":         "costly",
	"MANAGED_SERVICE":    "wire",
	"CROSS_AZ":           "wire",
	"SAME_AZ":            "good",
	"SAME_NODE":          "good",
	"PRIVATE_OFFCLUSTER": "good",
}

type clusterView struct {
	ClusterID    string
	LastSeenAgo  string
	FindingCount int
	CostHighINR  float64
	Sparkline    template.HTML
}

type findingView struct {
	FindingRow
	Tone string
}

type dashboardData struct {
	ClusterCount int
	FindingCount int
	TotalLow     float64
	TotalHigh    float64
	Clusters     []clusterView
	Findings     []findingView
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return pluralize(int(d.Minutes()), "min") + " ago"
	case d < 24*time.Hour:
		return pluralize(int(d.Hours()), "hr") + " ago"
	default:
		return pluralize(int(d.Hours()/24), "day") + " ago"
	}
}

func pluralize(n int, unit string) string {
	s := unit
	if n != 1 {
		s += "s"
	}
	return itoa(n) + " " + s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// handleDashboard renders the multi-cluster view (FR-I4/§13): every
// cluster's most recent snapshot side by side, a cost-trend sparkline per
// cluster, and the top findings ranked across all of them combined.
func handleDashboard(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		clusters, err := store.Clusters()
		if err != nil {
			log.Printf("dashboard: load clusters: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		findings, err := store.LatestFindings(50)
		if err != nil {
			log.Printf("dashboard: load findings: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		data := dashboardData{ClusterCount: len(clusters)}

		for _, c := range clusters {
			trend, err := store.CostTrend(c.ClusterID, 20)
			if err != nil {
				log.Printf("dashboard: cost trend for %s: %v", c.ClusterID, err)
			}

			data.Clusters = append(data.Clusters, clusterView{
				ClusterID:    c.ClusterID,
				LastSeenAgo:  relativeTime(c.LastSeen),
				FindingCount: c.FindingCount,
				CostHighINR:  c.CostHighINR,
				Sparkline:    sparklineSVG(trend, 200, 32),
			})

			data.FindingCount += c.FindingCount
			data.TotalLow += c.CostLowINR
			data.TotalHigh += c.CostHighINR
		}

		for _, f := range findings {
			tone := classTone[f.PathClass]
			if tone == "" {
				tone = "wire"
			}
			data.Findings = append(data.Findings, findingView{FindingRow: f, Tone: tone})
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := dashboardTemplate.Execute(w, data); err != nil {
			log.Printf("dashboard: render: %v", err)
		}
	}
}

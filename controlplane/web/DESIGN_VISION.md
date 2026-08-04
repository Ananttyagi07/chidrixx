# chidrixx — Design Vision & UI/UX Redesign Brief

*"Mission Control for your infrastructure's money."*

This is a creative brief, written to be reusable later as a build prompt.
It describes a complete reimagining of the chidrixx dashboard: not a
prettier version of the current sidebar-and-cards SaaS layout, but a
different kind of interface entirely — one built around a single strong
metaphor, with every screen, button, color, and motion decision
justified by that metaphor rather than borrowed from "what dashboards
usually look like."

Everything below is intentionally opinionated and specific — exact
pixel positions, exact hex values, exact library names — because a vague
brief ("make it feel premium and futuristic") produces a generic result.
A precise one doesn't.

---

## 0. North Star

**chidrixx doesn't show you a dashboard. It puts you in the control room
of your own infrastructure, watching real money move through real wires,
in real time.**

The current product (as of this document) is a competent, honest,
well-engineered SaaS dashboard: a fixed left sidebar, a topbar, a grid of
stat cards, donut charts, a line chart, a force-directed node graph
tucked into one page among sixteen. It is *correct*. It is also
*interchangeable* — swap the logo and it could be a dozen other cost or
observability tools. That interchangeability is the thing to kill.

The north star: **the topology of your infrastructure *is* the
interface**, not a chart buried on page six. You don't navigate to "Cost
Graph" to see your workloads talking to each other — that view *is* the
home screen, always live, always breathing, and every other page is a
lens you apply to it rather than a separate destination.

Everything a user could ever want to know — what's expensive, what's
new, what's broken, what the AI thinks, what a fix would do — surfaces
*inside* that living map, the way a mission control room puts every
signal on one wall of screens instead of forcing an operator to click
between sixteen tabs.

---

## 1. Explicit anti-goals

Naming what *not* to do matters as much as naming what to do — these are
the specific tropes this redesign must not fall into, because they are
exactly what makes AI-era design (and SaaS dashboards generally) start
to look identical:

- **No generic card grid as the home screen.** Stat tiles in a 4-up grid
  at the top of every SaaS product ever built. Numbers can still exist —
  they just don't get to be the *first* thing, or the *main* thing.
- **No purple-to-blue gradient hero, anywhere.** This is the single most
  recognizable "an AI designed this" tell in 2025–2026. Banned outright.
- **No neon-green-on-near-black "hacker terminal" cliché**, despite this
  being a genuinely technical product. That combination has become its
  own genre cliché, not a signal of technical seriousness.
- **No Inter/Space Grotesk as the "safe" typeface.** If it's the first
  font every AI-generated site reaches for, it's not the right choice
  here either.
- **No emoji as section markers.** Real iconography only.
- **No `rounded-lg` softness on every single surface.** Some edges in
  this UI should be sharp, engineered, precise — softness should be a
  deliberate choice on specific elements (the AI orb, notification
  toasts), not the default for everything.
- **No fixed always-expanded 16-item text sidebar.** It's the single
  most "generic dashboard" tell of the current build. It has to go.
- **No fabricated numbers, ever, in service of "looking impressive."**
  Every constraint this project has held itself to (§11 of
  `PROJECT_STATUS.md` — no invented dollar figures, no fake latency
  numbers, honest empty states) carries over unchanged. Unique visual
  language is not a license to fudge data. If a value is zero, it looks
  like zero, beautifully.
- **No sacrificing legibility for spectacle.** Every "living" visual
  element needs a plain, dense, keyboard-and-screen-reader-friendly
  fallback view sitting right next to it, not bolted on as an
  afterthought. Real operators debugging a real 2am cost spike need the
  boring table, fast, just as often as they want the dream.

---

## 2. The core metaphor: Mission Control

Everything downstream of this document should be checked against one
question: **"does this feel like a control room, or does this feel like
a dashboard?"**

The metaphor, concretely:

| Dashboard language (out) | Mission Control language (in) |
|---|---|
| Dashboard / Overview | The Deck |
| Cost Graph | The Grid — the living topology |
| Chart / node graph | Track — a real flight path between two systems |
| Anomaly | Flag |
| AI chat assistant | Ground Control |
| Notification | Signal |
| Applying a fix | Clearing a flag |
| Placement simulator | Re-routing |
| Dashboard refresh | Sync pulse |

This is not a re-skin exercise where the words change but the layout
stays the same — the *language* forces different layout decisions. A
"Grid" is something you look *at*, spatially, continuously. A
"Dashboard" is something you *read*, top to bottom, once. That's the
entire reason this redesign doesn't converge back onto a card grid.

**Visual reference points** (for tone, not to copy): a real air-traffic
control radar display; a modern rocket-launch mission control room (the
big shared wall, the small operator consoles); a submarine sonar
display; Bloomberg Terminal's information density *without* its dated
chrome; Linear's and Arc Browser's motion quality; Figma's infinite
pannable canvas as an interaction model, not a visual style.

---

## 3. Visual identity system

### 3.1 Color — "the HAL principle"

**Revised direction: light-mode-first, deliberately near-monochrome.**
No dark "glowing control room at night" — the reference shifted to
*2001: A Space Odyssey*'s spacecraft interiors: bright, white,
geometric, almost entirely without color, engineered rather than
atmospheric. Kubrick's crew used exactly one moment of real color in
that world — HAL's red eye — precisely because everything around it was
white, black, and silver. That's the rule here, named directly so it's
never forgotten while building: **the chrome is monochrome; color is
reserved entirely for real meaning, never decoration.** No glow, no
bloom, no soft colored light spilling off any element, anywhere.

The living Grid, the Ground Control orb, every button — all rendered in
ink/paper neutrals. The *only* time color appears is when it is a real,
computed signal: a path class on a real edge, a real critical/warning
status, a real savings figure. Because color never appears otherwise, it
reads as important the instant it shows up — the opposite of a UI where
every button is a different accent color and nothing stands out.

**Tokens:**

| Token | Hex | Use |
|---|---|---|
| `--field` | `#FAFAF8` | Page background — a very slightly warm off-white, not clinical pure white |
| `--paper` | `#FFFFFF` | Panel/card surface, sitting just above the field |
| `--paper-raised` | `#FFFFFF` | Elevated panels (Ground Control, modals) — differentiated by a slightly stronger border + shadow, never a color shift |
| `--line` | `#E5E4DF` | Borders, dividers — warm neutral gray |
| `--ink` | `#16160F` | Primary text — a warm near-black, not pure `#000` |
| `--ink-muted` | `#726F63` | Secondary text |
| `--ink-faint` | `#B7B4A8` | Disabled/placeholder text, rail icon default state |
| `--chrome` | `#16160F` | Primary action fill (buttons, active states) — **the "accent" is just ink itself**, not a separate color. A primary button is black text-on-white or white-on-black, not a colored pill. |

**Reserved real-signal colors** (never used for chrome/decoration, only
for actual computed data):

| Token | Hex | Use |
|---|---|---|
| `--x-az` | `#B5623F` | Cross-AZ traffic (the "expensive" real path class) — a muted clay/terracotta, desaturated, not a bright warning color |
| `--egress` | `#6B5E8C` | Internet-egress traffic — a muted, dusty plum |
| `--same-node` | `#4A6670` | Same-node / private-offcluster traffic — a muted slate-teal |
| `--critical` | `#B23B2E` | Real critical status only |
| `--warning` | `#B8863A` | Real warning status only |
| `--good` | `#3F7A5C` | Real success/savings only — a muted forest, not neon green |

All six are intentionally desaturated (max ~45% saturation) — considered
ink colors on paper, the way a technical diagram or a topographic map
uses color, never a bright/neon/glowing palette. Run each through a real
contrast + CVD check before shipping (see §8), not by eye.

**Dark mode** exists as a real secondary mode (re-stepped, not inverted):
`--field`→`#131310`, `--paper`→`#1B1B16`, `--ink`→`#F2F1EA` — same
neutral, near-monochrome philosophy, same reserved-color rule. It is not
the primary design target for this pass.

**No glow, ever.** Every place the earlier draft of this brief specified
a "soft breathing glow," a "beacon light," or a "warm inner glow" is
revised below to use *motion* (a scale pulse, an expanding-ring outline,
a shadow that deepens) instead of *light* (a blur/bloom/luminance
effect). Liveliness comes from precise, physics-based animation, not
from anything that looks like it emits light.

### 3.2 Typography

The product is already dependent on `@fontsource-variable/geist`
(confirmed in the current codebase) — lean *into* that instead of adding
a new typeface, and use it in a way almost no SaaS product does:

- **Display / headline face: Geist Mono**, at large scale (48–96px),
  bold, slightly negative letter-spacing at the largest sizes. Numbers
  and headlines set in mono at hero scale read as "instrument panel,"
  not "blog." This is the single highest-leverage typographic decision
  in this whole brief — most dashboards use a humanist sans for
  headlines; using mono boldly and large is genuinely uncommon and
  directly reinforces the control-room metaphor.
- **Body face: Geist Sans**, regular weight, for all prose, labels, and
  descriptions — keeps everything readable at normal density.
- **Data face: Geist Mono**, regular/medium, for every single number,
  cost figure, cluster ID, workload name, timestamp — tabular-nums
  always on. Numbers in this product should *never* be set in the sans
  face; consistent monospace numerals are what make a live-updating
  figure feel like instrumentation instead of copy.

**Type scale** (rem, 16px root):

| Role | Size | Weight | Face |
|---|---|---|---|
| Hero readout (e.g. total real-time spend) | 4.5–6rem | 700 | Geist Mono |
| Section headline | 1.75rem | 600 | Geist Mono |
| Panel title | 1rem | 600 | Geist Mono, uppercase, +0.04em tracking |
| Body | 0.9375rem | 400 | Geist Sans |
| Caption / meta | 0.75rem | 400 | Geist Sans, `--ink-muted` |
| Data value (inline) | 0.9375rem | 500 | Geist Mono, tabular-nums |

### 3.3 Iconography & shape language

- Custom line-icon set, 1.5px stroke, 24px grid, precise geometric
  construction (true circles, true right angles) — instrument/diagram
  iconography (radar sweep, relay, gauge), not the rounded-blob style
  most icon libraries default to. Icons are always solid `--ink` or
  `--ink-muted` — never colored, never with a colored background chip.
- Corner radius is a *coded signal*, not a blanket style: 0px (sharp) on
  data surfaces and the Grid's own panels — this is instrumentation, it
  should feel drafted, precise. A small radius (8px) is reserved
  specifically for conversational/AI surfaces (Ground Control panel) and
  toast notifications — a restrained, quiet softness signals "a presence
  talking to you," sharpness signals "an instrument reporting to you."
  Neither surface uses color or glow to make that distinction — shape
  alone carries it.
- Elevation is communicated with a **flat, precise drop-shadow only**
  (e.g. `0 1px 2px rgba(22,22,15,0.06), 0 4px 12px rgba(22,22,15,0.04)`)
  — never a colored or blurred glow-halo. A raised panel looks like a
  physical card lifted off the page, not a light source.

### 3.4 Sound (optional layer, off by default)

A single toggle in Settings: "Control-room audio." When on:
- A short, dry mechanical click on first load (the "power-on" sequence,
  §6) — think a relay switching, not a synth swell.
- A short two-tone chime when Ground Control proactively raises a real
  flag (ties to the already-shipped proactive anomaly watch).
- A subtle, dry, satisfying "click" when a fix is marked applied.
No sound ever plays without this being explicitly enabled first —
defaulting to silence is the only acceptable choice for a real work tool.

---

## 4. Global shell — exact layout

```
┌─────────────────────────────────────────────────────────────────────────┐
│  [chidrixx▾]     chidrixx-lab / checkout-service      [⌘K Search]   [●LIVE]  [◍ Ground Control]  [VE▾]  │  ← Top HUD, 72px
├────┬──────────────────────────────────────────────────────────────────────┤
│ ⊙  │                                                                      │
│ ▤  │                                                                      │
│ ⌁  │                    THE GRID                                         │
│    │           (living topology canvas)                                  │
│ ▦  │                                                                      │
│ ◈  │                                                                      │
│    │                                                                      │
│ ⚑  │                                                                      │
│    │                                                                      │
│ ⚙  │                                            [Map/List]  [⤓]  [Re-route]│
└────┴──────────────────────────────────────────────────────────────────────┘
  72px               rest of viewport
```

### 4.1 Top HUD bar — every element, exact

| Element | Position | Spec |
|---|---|---|
| Org/tenant switcher | 24px from left edge, vertically centered | Small mark (28px) + tenant name in Geist Sans 14px. Click → dropdown: switch tenant (if multiple), "Manage tenant." |
| Location breadcrumb | Immediately right of the org switcher, 16px gap | *Not* a page title. Reads as a location within the Grid: `chidrixx-lab / checkout-service` or just `All systems` at the top level. Click any segment to re-center the Grid on it. Geist Mono, `--ink-muted`, 13px. |
| Command Search | Horizontally centered | 320px-wide pill, `--paper` background, 1px `--line` border, placeholder text `Search or ask… (⌘K)`. Click or `⌘K`/`Ctrl K` anywhere in the app opens the full Command Palette (§4.4). |
| Live status pulse | Right of center, 24px gap from search | Kept from the current build deliberately — it's honest and it works. `● LIVE · syncing every 15s` in `--ink-muted`; the dot itself is the one place allowed a small expanding-ring motion (an outline ring scaling out and fading, `--ink` colored, no blur) instead of the old colored ping-glow — liveliness via precise motion, not light. |
| Ground Control orb | 24px right of the live pulse | 36px circle, 1px `--line` border, `--paper` fill — flat, no glow. Idle: a thin outline ring slowly expands from the circle's edge and fades (4s cycle) — a sonar-ping motion, not a light bloom. Has-a-flag: a small solid `--ink` dot badge upper-right of the circle, and the ring cycle quickens to 2s. Click → Ground Control panel slides in from the right (§5.3). This single control **replaces both** the old separate "chat assistant" nav item and the anomaly notification bell — one living presence instead of two bolted-on icons. |
| Account | Far right, 24px from edge | 28px avatar circle with initials, click → small menu: role label, "Log out." De-emphasized on purpose — this is utility, not a focus point. |

### 4.2 Left Command Rail — every element, exact

72px wide, collapsed by default; hovering anywhere on the rail expands it
to 240px (labels fade in to the right of each icon) after a 150ms delay;
a pin icon at the bottom keeps it expanded permanently for anyone who
prefers that. This replaces the current always-expanded 16-item text
sidebar with something that gets out of the way until it's wanted.

Icons are grouped into four constellations with an 20px gap between
groups (not sixteen items in one undifferentiated list):

| Group | Items | Icon direction |
|---|---|---|
| **Observe** | The Grid (home), Workloads, Costs & Usage | A radar-sweep glyph for the Grid specifically — it should look different from every other rail icon, since it's the home/anchor destination |
| **Understand** | Forecasting, Flags (was: Anomalies — historical list view), History | A trend-line glyph, a flag glyph, a clock-arc glyph |
| **Act** | Re-routing (was: Automations/remediation), Savings Advisor, Budgets | A relay/switch glyph, a shield glyph, a gauge glyph |
| **Organize** | Teams, Reports | A network-of-people glyph, a document glyph |
| *(bottom-pinned, separated by a divider)* | Settings, rail pin/unpin toggle | Standard gear; a thumbtack |

Each icon: 44×44px hit target, centered icon at 20px. **Active state**:
a 2px `--ink`-colored bar on the rail's left inner edge next to that
icon, plus the icon itself brightens from `--ink-faint` to `--ink` — no
filled rounded-rectangle background pill (that's the generic-sidebar
tell being explicitly avoided). **Hover state**: icon brightens
partially, tooltip label appears 8px to the right if the rail is
collapsed.

### 4.3 Ground Control panel (the AI, expanded)

Slides in from the right as a 420px overlay (not a page navigation — the
Grid keeps running live underneath, dimmed to 60% opacity through a
scrim). Structure top to bottom:

1. **Header** (56px): "Ground Control" in Geist Mono uppercase, a small
   live-status dot, a close `×` at the far right (44×44 hit target).
2. **What Ground Control is watching** (collapsible, collapsed by
   default after first visit): a compact real readout — this is where
   the AI evaluation telemetry (real success rate, real tool-call
   reliability) surfaces, reframed as "Ground Control's own vitals"
   rather than a separate settings-style stats page. Honest empty state
   if no real requests have happened yet.
3. **Active flags** (if any): real proactively-detected anomalies,
   each as a compact row — cluster, the real cost jump, a "Clear" and a
   "View on Grid" action (the latter closes the panel and flies the Grid
   camera to that node).
4. **Conversation** (the existing real tool-calling chat, unchanged
   underneath): message list, suggestion chips for a first-time user,
   input field pinned to the bottom of the panel.

### 4.4 Command Palette (⌘K)

A new, genuinely modernizing interaction layer this product doesn't have
yet (in the Linear/Raycast/Arc tradition) — and a real usability
improvement, not just decoration, for a product with sixteen-plus
sections and hundreds of real workloads to jump between.

- Centered overlay, 640px wide, opens with a 120ms scale+fade-in,
  backdrop blurred and dimmed to 40%.
- Auto-focused search input at the top, results below, grouped:
  **Go to** (any section), **Jump to** (fuzzy-matched real workload/
  cluster names from live data), **Do** (`Mark [x] as applied`,
  `Ask Ground Control: why did cost spike`, `Simulate 3-zone placement`).
- Arrow keys navigate, `Enter` selects, `Esc` closes. Recent selections
  pinned to the top on next open.
- Library: `cmdk` (small, purpose-built, already the de facto standard
  for exactly this pattern).

---

## 5. The Grid — the living topology canvas (replaces "Overview")

This is the centerpiece and the highest-effort, highest-payoff piece of
this whole brief.

**What it is:** the existing real force-directed topology graph
(`graphLayout.ts` — dependency-free, already built) evolved from "a page
you visit" into "the thing that's always running behind everything,"
rendered large, full-canvas, with a materially richer visual treatment:

- **Nodes** = real workloads. Drawn as precise thin-stroke circles
  (`--ink` outline, `--paper` fill — never a filled color blob), radius =
  real cost (log-scaled, so one enormous spender doesn't visually erase
  everything else). A live node's outline has a **slow, subtle scale
  pulse** (1.0→1.02→1.0, no blur, no color shift) whose cycle rate
  reflects real ingest cadence for that cluster — liveliness via precise
  motion, never via glow.
- **Edges (Tracks)** = real byte flows between two real endpoints.
  Rendered as a thin `--ink-faint` line with **small solid dots
  traveling along it** (like an animated transit-line indicator) —
  dot density/speed maps to real bytes/sec, not decoration. The dots
  (not the line itself) carry color, and only the dots: cross-AZ traffic
  in `--x-az` (this is the expensive, real "hot" path), internet egress
  in `--egress`, same-node/private-offcluster in `--same-node` — the
  same real categories the product already computes, now finally given a
  visual language equal to their real meaning, without turning the whole
  canvas into a colored light show.
- **Zoom & pan**: a real infinite canvas (Figma-style trackpad/scroll
  zoom, drag to pan). Zoomed all the way out: every cluster, as
  clustered constellations. Zoom into one: its workloads spread out.
  Click a node: the camera flies to center on it (a real eased camera
  animation, ~500ms) and a contextual detail panel slides up from the
  bottom-right — real trend sparkline, real fix hint if one exists, a
  "Mark as applied" action — *without leaving the Grid*.
- **Re-routing (the placement simulator, made spatial)**: toggling
  "Re-route" (bottom-right floating control) overlays translucent
  concentric rings on the canvas representing real availability zones.
  Dragging a node into a different ring is a **direct, live
  re-computation** of the existing real `OptimizePlacement` math — the
  projected-savings number updates live as you drag, before you drop.
  This turns an existing real feature (currently a dropdown + stat
  tiles) into a genuinely novel direct-manipulation interaction that
  only makes sense once the topology *is* the interface.

**The essential parallel mode — List view:** a toggle in the bottom-right
floating cluster (§4, "Map/List") switches the same real data into a
dense, fully keyboard-navigable, screen-reader-friendly table — every
column sortable, every row real. This is not a lesser fallback bolted on
for compliance; it is treated as equally first-class, because a real
operator debugging a real 2am cost spike needs fast, boring, precise
data at least as often as they want to watch particles flow. The
transition between Map and List is a real **morph animation** — nodes
literally reflow into table rows via Framer Motion's `layout` position
tracking — rather than a hard cut, so the two modes feel like one
continuous system rather than two disconnected pages.

**Bottom-right floating control cluster** (persists across every Grid
view):

| Control | Spec |
|---|---|
| Map / List toggle | Two-state pill switch, 96px wide |
| Download snapshot | Icon button, 36px — the existing real "download as JSON" feature, relocated here |
| Re-route | Secondary-style pill button, opens the ring-drag placement mode described above |

---

## 6. Motion system

- **First-load power-on sequence** (once per session, skip on any
  keypress/click, and skipped entirely under `prefers-reduced-motion`):
  the HUD bar draws in from center-outward like a technical-drawing
  reveal — a thin `--ink` rule sweeping out to full width (~400ms, GSAP)
  — then the Grid's nodes appear in staggered sequence, most-recently-
  active first, each simply fading + scaling in from 0.95→1 (~800ms). No
  colored light sweep, no bloom — a precise line-draw reveal, like a
  blueprint being drafted. Optional dry mechanical click if audio is
  enabled (§3.4).
- **Number counters**: every real figure that changes (spend, savings,
  a node's cost on hover) animates via a spring-based count (Framer
  Motion `useSpring`), never a hard snap — reinforces "this is alive,"
  and is cheap to implement given Framer Motion is already a dependency.
- **Hover physics**: primary CTAs only (not every button — restraint
  matters here) get a subtle magnetic cursor-proximity pull, in the
  Linear/Arc marketing-site tradition. Strictly disabled under
  `prefers-reduced-motion`, falling back to a plain opacity/scale hover.
- **Ambient pulse**: any element reflecting genuinely live data (the
  HUD's live dot, Grid nodes, Ground Control's idle state) gets a slow
  (2–4s cycle) **scale or outline-ring** pulse — never a color/opacity
  glow — and never faster, never on static/historical data, so the pulse
  reliably signals "this is live" throughout the product.
- **Toasts/signals**: when Ground Control raises a real flag, it doesn't
  just badge the orb — a small toast slides in from the top-right, 8px
  radius (the one deliberately soft-cornered surface, §3.3), flat
  drop-shadow only, auto-dismissing after 6s or on click (which opens
  Ground Control focused on that flag).
- **Parallax, used sparingly and only where it's true**: on the Grid,
  panning the canvas moves the node layer and a faint background
  reference-grid (a thin dot-grid, like graph paper) at slightly
  different rates — real depth cueing for a real spatial canvas, not
  decorative scroll-parallax bolted onto a marketing page. Nowhere else
  in the product needs parallax, and it shouldn't be forced elsewhere.

---

## 7. Section-by-section: every other page, reimagined

Each existing page keeps its real underlying data and logic — this
section only changes *how it's framed and laid out*, consistent with the
Mission Control language.

### 7.1 Costs & Usage / Budgets
Reframed as **"Fuel"** — the resource-consumption ledger. Kept
list-first (dense tables, real sortable columns) rather than
canvas-first: this is inherently tabular financial data, and the Grid
above already gives the spatial view. Budget threshold becomes a real
horizontal gauge with a `--ink`-colored fill (switching to `--warning`/
`--critical` only if the real threshold is actually approached/crossed)
and a marker line for the configured limit — an instrument reading, not
a progress-bar-in-a-card.

### 7.2 Forecasting
Reframed as **"Trajectory."** The existing real backtested Holt/damped-
Holt model's output becomes a single large mono-numeral hero readout
("in 14 days: ₹X") with the real confidence band rendered as a soft
gradient trail behind the trend line, echoing the Grid's particle-trail
visual language rather than a standalone Recharts-default line chart.

### 7.3 Flags (was: Anomalies)
The historical list of every real anomaly ever detected — Ground
Control (§4.3) is where *new* ones are proactively surfaced, but this
page is the full, dense, filterable real log. Table-first, List-view
styled identically to the Grid's own List mode for consistency.

### 7.4 Re-routing (was: Automations)
The dry-run remediation preview and the placement simulator both live
here as the "deep dive" versions of what's directly manipulable on the
Grid — every real "would apply / would skip" decision as a dense real
list, with the exact same reasons already computed, just restyled to
match. The generated NetworkPolicy manifests keep their existing
copy/download actions unchanged (they work; don't reinvent a solved
problem for its own sake).

### 7.5 Teams / Savings Advisor / History / Workloads / Reports
Kept as focused, list-first utility pages — not every page needs the
spectacle treatment, and forcing it everywhere is exactly the kind of
gimmick-fatigue this brief explicitly warns against elsewhere. They get
the new type system, color tokens, and rail/HUD shell, and stop there.

### 7.6 Settings
Where the "Control-room audio" toggle (§3.4), reduced-motion override,
and rail pin/unpin preference live, alongside existing real settings
(budget threshold, team invites).

---

## 8. Accessibility & performance guardrails

- `prefers-reduced-motion` is checked once, globally, and turns off: the
  power-on sequence, magnetic hover, particle animation on Grid edges
  (rendered as static gradient lines instead), and number count-up
  (values update instantly). This is a hard requirement, not a nice-to-have.
- List view is a complete, first-class, keyboard-and-screen-reader-
  accessible parallel to every canvas-based view — never a degraded
  afterthought.
- Real perf budget for the Grid: cap simultaneously-rendered particle
  streams (reuse the existing pattern of capping by real cost-rank, the
  same principle already used for `dashboardSummaryFindingsLimit`),
  degrade to static edges beyond that cap rather than dropping frames.
- Color contrast: every token pair above meets WCAG AA at minimum for
  text; the signal/current/offworld triad was chosen to additionally
  stay distinguishable under common color-vision deficiencies (verify
  with a real contrast/CVD checker before shipping, not by eye).

---

## 9. Concrete tech stack (specific, not vague)

| Purpose | Library | Note |
|---|---|---|
| Page/element motion, counters, hover physics | **Framer Motion** | Already a dependency — no new cost |
| Power-on sequence, any scroll-driven reveals | **GSAP** | Already a dependency |
| The Grid's force layout | Existing `graphLayout.ts` | Already built, dependency-free — extend, don't replace |
| Grid particle-stream rendering | Hand-rolled `<canvas>` RAF loop, or **tsParticles** if hand-rolling proves too costly | Keep it 2D/canvas, not full WebGL/Three.js — real perf and accessibility win over a heavier 3D approach for this data shape |
| Command palette | **cmdk** | New, small, purpose-built |
| Smooth-scroll physics on any long list panel | **Lenis** | New, lightweight |
| A few more react-bits components (already vendored: DecryptedText, RotatingText, VariableProximity) | e.g. a `SplitText` reveal for section headlines, a `Magnet` wrapper for primary CTAs | Pull in individually via the existing `shadcn` CLI pattern, not wholesale |
| AI orb idle/alert/talking states, if pursuing studio-quality micro-animation | **Rive** | Optional — a well-executed CSS/Framer Motion expanding-ring pulse (no glow) is a legitimate, lower-effort alternative that still meets the brief |
| Optional sound layer | **Howler.js** | Tiny footprint, off by default |
| Typography | `@fontsource-variable/geist` | Already a dependency — Geist Mono for display+data, Geist Sans for body |

---

## 10. Phased build roadmap (honest)

This is a large redesign. Recommended sequencing, each phase shippable
and real on its own:

1. **Phase 1 — Identity + shell.** New color tokens, type scale, the Top
   HUD and Left Rail exactly as specified, the Command Palette. Every
   existing page keeps its current internal layout for now, just
   re-skinned and re-homed under the new shell. Lower risk, immediately
   visible, no changes to any data logic.
2. **Phase 2 — Motion pass.** Number counters, hover physics, the
   power-on sequence, ambient pulses — layered onto the Phase 1 shell.
3. **Phase 3 — The Grid.** The full living-topology centerpiece,
   including Re-routing-as-direct-manipulation and the Map/List morph.
   This is the highest-effort, highest-payoff, highest-risk phase —
   worth prototyping in isolation (a throwaway branch/route) against
   real production-scale data before committing, the same discipline
   this project has applied to every other real feature so far.
4. **Phase 4 — Section reframing.** Rename/restyle the remaining pages
   per §7, folding in Lenis/react-bits polish where it earns its place.

---

## 11. Condensed prompt (paste-ready)

> Redesign chidrixx as "Mission Control for infrastructure cost," not a
> SaaS dashboard — a bright, white, precise, *2001: A Space Odyssey*
> spacecraft-interior aesthetic, not a neon night-time control room.
> Replace the home screen with a living, always-running topology canvas
> ("the Grid"): real workloads as precise thin-stroke circles sized by
> real cost, real byte-flows as small solid dots traveling along thin
> lines, colored by path class. Replace the sidebar with a slim icon-only
> command rail (72px, expands on hover, grouped into Observe/Understand/
> Act/Organize). Replace the chat-assistant nav item and the
> notification bell with one living presence — a flat circular orb in
> the top HUD called Ground Control, marked live by a slow expanding-ring
> pulse (never a glow), which proactively raises real flags and answers
> real questions in a slide-in panel. Add a `⌘K` command palette.
> Color system: near-monochrome — warm off-white field, warm near-black
> ink, no separate "accent" color for chrome or buttons at all. Reserve
> color *exclusively* for real computed signals: a muted clay for
> cross-AZ traffic, a muted plum for internet egress, a muted slate-teal
> for same-node traffic, muted red/amber/green only for real critical/
> warning/success status. Never a gradient, never a glow, never a
> bloom/blur light effect anywhere in the product. Typography: Geist Mono
> at large scale for headlines and every single number in the product;
> Geist Sans for body text only. Motion carries all the "alive" feeling
> that color and light aren't allowed to: every live element pulses via
> scale or an expanding outline ring, never opacity-glow; every changing
> number counts up with spring physics; magnetic hover on primary
> buttons; a real morph transition between the Grid and its full,
> first-class, keyboard-accessible List view, never a lesser fallback.
> Respect `prefers-reduced-motion` everywhere, without exception. Never
> fabricate a number to make the visuals more impressive than the real
> data.

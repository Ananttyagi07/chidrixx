# chidrixx — YC Interview Prep

_Built 2026-08-05, updated 2026-08-06. Every number here is MEASURED from the real system and
traceable to PROJECT_STATUS.md or PRICING.md. Do not quote a number in an
interview that isn't in this file — if you can't trace it, don't say it._

---

## 0. Format reality — read this first

The YC interview is **~10 minutes**, rapid-fire, with partners
**interrupting you**. It is not a presentation. There are no slides.

**What actually gets tested:**
- Can you answer in **1–2 sentences**, not paragraphs
- Do you know your numbers **cold**, without hedging
- Do you say **"I don't know"** cleanly instead of bluffing
- Is there **evidence** anyone wants this

**What does NOT get tested:** the depth of your eBPF knowledge. They will
not out-technical you and they don't care to. Depth only matters if they
follow up — and they only follow up if your short answer was interesting.

**The single most likely way you fail:** giving a 90-second technical
answer to a 10-second business question. Practice being interrupted.

---

## 1. Numbers you must know cold

If you fumble any of these, it reads as "he didn't build this."

### Product / technical
| Fact | Number |
|---|---|
| Byte accuracy vs. real `iperf3` 1GB transfer | **0.156% difference** (real TCP/IP overhead, not error) |
| Agent CPU overhead | **4m CPU = 0.05%** (was 133m/1.66% before the kubectl-exec fix) |
| Load-tested concurrent flows | **10,543** at 27m CPU (0.34%) |
| Real rows in live production DB | **4,125,883** |
| Total hand-written code | **~15,400 lines** Go + TypeScript |
| Backend tests / E2E tests | **219 Go** (+29 agent) / **28 Playwright** |
| Real API endpoints | **21** |
| Path classes classified | **8** (same-node → cross-AZ → cross-region → NAT → internet) |

### Business / unit economics
| Fact | Number |
|---|---|
| Real COGS to serve one cluster | **$1.57/month** (₹132) |
| Gross margin at proposed pricing | **87–90%** |
| Real cost per AI chat turn | **$0.0023** (3,752 real tokens, measured) |
| Proposed price | **$5/node/month** |
| Kubecost's public price | **$8/node/month** |
| Storage share of COGS | **78–94%** |
| Paying customers today | **Zero. Say it plainly.** |

### Market
| Fact | Number |
|---|---|
| Container users running K8s in production | **82%** (CNCF 2025 survey) |
| Fortune 100 running K8s in production | **77%** |
| K8s adopters that are 1,000+ employees | **91%** |
| K8s tooling market | **$3.13B (2026) → $8.41B (2031)**, 21.85% CAGR |

---

## 2. The killer questions — with honest answers

### "How many users do you have?"
> **"Zero. I've been proving the hard part works — byte-accurate kernel-level
> attribution, verified against real transfers. I'm now taking it to operators."**

Do **not** dress this up. They know. Owning it fast buys credibility for
everything after. Then immediately pivot to what you *have* proven.

### "Why hasn't anyone done this?"
> **"Kubecost does classify cross-zone and cross-region traffic — I want to be
> accurate about that. What nobody does is close the loop: generate the actual
> NetworkPolicy fix and prove it won't break anything else before you apply it."**

⚠️ **Never claim Kubecost can't see network cost.** They ship a
`network-costs` DaemonSet that classifies internet/cross-region/cross-zone
egress per pod. If you claim otherwise and a partner knows, you lose the
room. Your real edge is the **remediation loop**, not the measurement.

### "Why are you solo?"
Have a real answer ready. This is your biggest structural weakness and
they will ask. Don't be defensive; don't over-explain. Say what's true,
and say whether you're looking.

### "What's your unfair advantage?"
> **"I have a production eBPF agent that measures real traffic at 0.05% CPU,
> verified byte-accurate. That took months and most people building cost tools
> won't go to kernel level."**

### "Isn't this too specific? / How big can this get?"
> **"Network cost is the wedge, not the company. The asset is a kernel-level
> agent that already maps every workload-to-workload flow in a cluster. Cost is
> the first thing we pointed it at — the same data does egress security and
> service dependency mapping."**

Be ready for: *"So why aren't you doing security, where budgets are bigger?"*
That's a fair hit. Honest answer: cost is where you can prove ROI in a
demo; security is a longer sale for an unknown vendor.

### "What's stopping a customer from deploying this today?"
> **"Nothing technical that I know of. It used to need `privileged: true` —
> now it runs `privileged: false`, drops ALL capabilities and adds exactly
> two, `CAP_BPF` and `CAP_NET_ADMIN`. I minimised that by testing, not
> guessing: I had `CAP_PERFMON` in there too, removed it, and confirmed the
> programs still load and count real flows without it."**

If pushed on PodSecurity, **do not overclaim** — the honest answer wins:

> **"It still doesn't pass `baseline` or `restricted` — neither allows adding
> those capabilities, and both forbid the hostPath cgroup mount. No eBPF
> agent can. That namespace needs an exemption whichever vendor you pick.
> What changed is that it's now a reviewable ask instead of a blanket
> `privileged: true`."**

Knowing the exact remaining limit of your own fix is a strength signal.

### "What have you learned from users?"
If the honest answer is still "I haven't talked to any" — **fix this before
you apply.** There is no good version of this answer without conversations.

### "How do you make money?"
> **"$5/node/month — deliberately under Kubecost's $8. Real COGS is $1.57 per
> cluster, so gross margin is ~88%."**

### "What's your biggest risk?"
> **"That security teams won't allow a kernel-level agent from an unknown
> vendor at all. I've removed the technical reasons to say no — two
> capabilities, no privileged mode, aliased data to the LLM. What's left is
> pure trust, and that's what I need customers and this batch for."**

---

## 3. The four hats — the one thing each must own

You said CEO/CTO/CMO/CFO. YC doesn't want four answers; they want to see
you know which number matters per hat.

- **CEO — the vision sentence.** "Kernel-level network intelligence for
  Kubernetes; cost is the first application." One sentence. Never longer.
- **CTO — the moat sentence.** "eBPF byte-accurate attribution at 0.05%
  CPU, plus a remediation loop that validates fixes against real traffic
  before recommending them."
- **CMO — the wedge customer.** "Teams on EKS with a visible cross-AZ
  line item they can't attribute to a workload." Be specific about *who*,
  not *what*.
- **CFO — three numbers.** $1.57 COGS, 88% gross margin, $5/node price.
  That's the whole finance answer at this stage. Don't build a 5-year model.

---

## 4. Technical depth — for follow-ups only

Use these **only if asked**. Volunteering them is how you lose the room.

- **How it measures:** `cgroup_skb` eBPF programs (egress + ingress) count
  bytes per `(cgroup_id, 5-tuple)` in a `BPF_MAP_TYPE_LRU_PERCPU_HASH`.
  Both programs unconditionally `return 1` — they **only count, never
  gate traffic**. That's why a crashed agent can't break the cluster
  (proven: killed the pod mid-traffic, 45 requests, 0 failures).
- **Why not payload inspection:** it never reads packet contents. Only
  byte counters per flow tuple. This is the answer to every privacy
  question and it's auditable in ~60 lines of C.
- **How it prices:** 8 path classes × confidence level, with the estimate
  band widening (15/35/60%) when topology data is incomplete — rather
  than guessing a precise wrong number.
- **The remediation loop:** generate NetworkPolicy → replay it against
  real recorded traffic → if another workload in that namespace depends on
  the blocked destination, disqualify it with the specific workload named.
- **Known limitation, state it freely:** doesn't work on kind/k3d
  (Docker-in-Docker cgroup namespace `EPERM`). Real EKS/GKE/AKS is fine.

**Bugs you should be proud to mention if asked "what went wrong":**
- BPF map capped at 4096, silently LRU-evicting — undercounted 9.7k real
  flows as 3.6k. Found by load testing, fixed, now has a saturation alert.
- Added a covering index that made an unrelated read path **10x slower**
  (3.9s → 36-45s) via write amplification. Caught by measuring live,
  reverted, fixed by scoping the query instead.

These are strong answers. They show you measure rather than assume.

---

## 5. When you don't know

Say: **"I don't know — here's how I'd find out."** Then stop talking.

Partners deliberately probe past your knowledge to see what you do at the
edge. Bluffing is the fastest way to lose. You have a genuine track record
of catching your own wrong assumptions — that instinct is the asset.

---

## 6. Do this before applying (in priority order)

1. **Talk to 5 real EKS operators.** Nothing else on this list comes close.
   It fixes the single worst answer in the application, and it is the only
   item here that cannot be solved by writing code.
2. **Have a real answer for "why solo."**
3. **Practice the 10-minute format**, out loud, being interrupted.

**Done since this doc was written** (2026-08-06) — the technical blockers
that used to sit at #2 and #3 are closed:

- ✅ **Agent privileges narrowed.** `privileged: false`, `drop: ALL`, two
  capabilities. Empirically minimised (`CAP_PERFMON` tested and removed)
  and verified live on a 6.17 kernel.
- ✅ **Published artifacts current.** Charts and images at `0.2.0`,
  `image.tag` derives from the chart's `appVersion` so they can't drift,
  and a release workflow re-verifies anonymous installability.
- ✅ **AI data boundary.** Identifiers pseudonymised and generated
  manifests dropped before anything reaches Groq, default-on, with
  per-request audit logging.

That's worth saying in the interview *only if asked* — it's evidence of
velocity, not of demand. Do not let it substitute for #1.

---

## 7. Honest self-assessment

**What's genuinely strong:** a working, verified, technically deep product
built solo; unusual engineering rigor; a documented honesty audit most
companies would never publish; real measured unit economics; and a
deployment story that now survives a security review instead of ending it.

**What's genuinely weak:** zero users, zero revenue, zero user
conversations, solo founder, and a crowded category with an IBM-backed
incumbent.

Note what changed and what didn't. Three technical weaknesses came off
that second list in two days. The four that decide the outcome are all
still there, and none of them are engineering problems. That asymmetry is
the single most important thing to understand about your own position.

Both lists are true. Bring both. The founders who get in are usually the
ones who describe the second list more accurately than the partners could.

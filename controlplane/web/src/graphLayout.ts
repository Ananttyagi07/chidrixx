// A minimal force-relaxation layout -- deliberately not a physics engine
// or a new dependency, just repulsion between every node pair, a spring
// pulling connected nodes together, and a gentle pull toward center, run
// for a fixed number of iterations and rendered as its final settled
// state (references/anti-patterns.md warns against generative graphics
// via hand-authored SVG paths; this produces plain circles+lines, not
// path data, so plain SVG is the right tool here).

export interface GraphNode {
  id: string;
  x: number;
  y: number;
  vx: number;
  vy: number;
}

export interface GraphEdgeInput {
  source: string;
  target: string;
}

const REPULSION = 12000;
const SPRING_LENGTH = 160;
const SPRING_STRENGTH = 0.02;
const CENTER_PULL = 0.01;
const DAMPING = 0.85;

export function layoutGraph(
  nodeIDs: string[],
  edges: GraphEdgeInput[],
  width: number,
  height: number,
  iterations = 300,
): Map<string, { x: number; y: number }> {
  const cx = width / 2;
  const cy = height / 2;

  const nodes = new Map<string, GraphNode>();
  nodeIDs.forEach((id, i) => {
    // Seed on a circle so the relaxation starts from a stable, non-
    // overlapping arrangement rather than a random pile in one corner.
    const angle = (i / Math.max(nodeIDs.length, 1)) * Math.PI * 2;
    const r = Math.min(width, height) * 0.3;
    nodes.set(id, {
      id,
      x: cx + r * Math.cos(angle),
      y: cy + r * Math.sin(angle),
      vx: 0,
      vy: 0,
    });
  });

  for (let iter = 0; iter < iterations; iter++) {
    const list = Array.from(nodes.values());

    for (let i = 0; i < list.length; i++) {
      for (let j = i + 1; j < list.length; j++) {
        const a = list[i];
        const b = list[j];
        let dx = a.x - b.x;
        let dy = a.y - b.y;
        let distSq = dx * dx + dy * dy;
        if (distSq < 1) distSq = 1;
        const dist = Math.sqrt(distSq);
        const force = REPULSION / distSq;
        dx = (dx / dist) * force;
        dy = (dy / dist) * force;
        a.vx += dx;
        a.vy += dy;
        b.vx -= dx;
        b.vy -= dy;
      }
    }

    for (const e of edges) {
      const a = nodes.get(e.source);
      const b = nodes.get(e.target);
      if (!a || !b) continue;
      const dx = b.x - a.x;
      const dy = b.y - a.y;
      const dist = Math.max(Math.sqrt(dx * dx + dy * dy), 1);
      const displacement = dist - SPRING_LENGTH;
      const force = displacement * SPRING_STRENGTH;
      const fx = (dx / dist) * force;
      const fy = (dy / dist) * force;
      a.vx += fx;
      a.vy += fy;
      b.vx -= fx;
      b.vy -= fy;
    }

    for (const n of list) {
      n.vx += (cx - n.x) * CENTER_PULL;
      n.vy += (cy - n.y) * CENTER_PULL;
      n.vx *= DAMPING;
      n.vy *= DAMPING;
      n.x += n.vx;
      n.y += n.vy;
    }
  }

  const out = new Map<string, { x: number; y: number }>();
  for (const n of nodes.values()) {
    out.set(n.id, { x: n.x, y: n.y });
  }
  return out;
}

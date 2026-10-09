export interface GraphNode { id: string; name: string; knowledge_id: string; attributes: string[] }
export interface GraphEdge { id: string; source: string; target: string; type: string }
export interface KnowledgeGraph { nodes: GraphNode[]; edges: GraphEdge[]; truncated: boolean; state: 'ready' | 'empty' | 'disabled' }
export type PlacedNode = GraphNode & { x: number; y: number }
export const emptyGraph = (): KnowledgeGraph => ({ nodes: [], edges: [], truncated: false, state: 'empty' })
export function canBrowseGraph(role: string | null | undefined) { return ['owner', 'admin', 'contributor'].includes(role || '') }

export function edgeGeometry(edge: GraphEdge, nodes: Map<string, PlacedNode>, edges: GraphEdge[]) {
  const a = nodes.get(edge.source), b = nodes.get(edge.target)
  if (!a || !b) return { path: '', x: 0, y: 0 }
  const siblings = edges.filter(e => [edge.source, edge.target].includes(e.source) && [edge.source, edge.target].includes(e.target))
  const lane = siblings.findIndex(e => e.id === edge.id)
  if (a.id === b.id) {
    const radius = 55 + lane * 22
    return { path: `M ${a.x - 10} ${a.y - 18} C ${a.x - radius} ${a.y - radius * 2}, ${a.x + radius} ${a.y - radius * 2}, ${a.x + 10} ${a.y - 18}`, x: a.x, y: a.y - radius * 1.5 }
  }
  // Use a stable orientation for reciprocal edges so their arcs separate too.
  const direction = a.id < b.id ? 1 : -1
  const distance = Math.max(1, Math.hypot(b.x - a.x, b.y - a.y)), offset = (lane - (siblings.length - 1) / 2) * 65 * direction
  const cx = (a.x + b.x) / 2 - (b.y - a.y) / distance * offset, cy = (a.y + b.y) / 2 + (b.x - a.x) / distance * offset
  return { path: `M ${a.x} ${a.y} Q ${cx} ${cy} ${b.x} ${b.y}`, x: (a.x + b.x) / 4 + cx / 2, y: (a.y + b.y) / 4 + cy / 2 - 7 }
}

// A browser session is bounded too: repeated expansions cannot accumulate an
// unbounded graph. IDs are Neo4j element IDs, never entity names (homonyms exist).
export function mergeGraph(current: KnowledgeGraph, next: KnowledgeGraph): KnowledgeGraph {
  const all = new Map([...current.nodes, ...next.nodes].map(n => [n.id, n]))
  const nodes = [...all.values()].slice(0, 240), ids = new Set(nodes.map(n => n.id))
  const edges = [...new Map([...current.edges, ...next.edges].map(e => [e.id, e])).values()]
    .filter(e => ids.has(e.source) && ids.has(e.target))
  return { nodes, edges: edges.slice(0, 480), state: nodes.length ? 'ready' : next.state,
    truncated: current.truncated || next.truncated || all.size > 240 || edges.length > 480 }
}

export function layoutGraph(graph: KnowledgeGraph): PlacedNode[] {
  const nodes = graph.nodes.map((n, i) => ({ ...n, x: 500 + 230 * Math.cos(i * 2.39996), y: 310 + 200 * Math.sin(i * 2.39996) }))
  const byID = new Map(nodes.map(n => [n.id, n]))
  for (let step = 0; step < 100; step++) {
    const forces = nodes.map(n => ({ x: (500 - n.x) * 0.012, y: (310 - n.y) * 0.012 }))
    for (let i = 0; i < nodes.length; i++) for (let j = i + 1; j < nodes.length; j++) {
      const a = nodes[i]!, b = nodes[j]!, dx = a.x - b.x || 0.1, dy = a.y - b.y || 0.1
      const length = Math.max(12, Math.hypot(dx, dy)), force = 1700 / (length * length)
      forces[i]!.x += dx / length * force; forces[i]!.y += dy / length * force
      forces[j]!.x -= dx / length * force; forces[j]!.y -= dy / length * force
    }
    for (const e of graph.edges) {
      const a = byID.get(e.source), b = byID.get(e.target)
      if (!a || !b) continue
      const dx = b.x - a.x, dy = b.y - a.y, d = Math.max(1, Math.hypot(dx, dy)), f = (d - 145) * 0.015
      const ai = nodes.indexOf(a), bi = nodes.indexOf(b)
      forces[ai]!.x += dx / d * f; forces[ai]!.y += dy / d * f
      forces[bi]!.x -= dx / d * f; forces[bi]!.y -= dy / d * f
    }
    nodes.forEach((n, i) => { n.x = Math.max(55, Math.min(945, n.x + Math.max(-10, Math.min(10, forces[i]!.x)))); n.y = Math.max(55, Math.min(565, n.y + Math.max(-10, Math.min(10, forces[i]!.y)))) })
  }
  if (nodes.length) {
    const xs = nodes.map(n => n.x), ys = nodes.map(n => n.y)
    const minX = Math.min(...xs), maxX = Math.max(...xs), minY = Math.min(...ys), maxY = Math.max(...ys)
    const scale = Math.min(2.5, 790 / Math.max(1, maxX - minX), 400 / Math.max(1, maxY - minY))
    nodes.forEach(n => { n.x = 500 + (n.x - (minX + maxX) / 2) * scale; n.y = 310 + (n.y - (minY + maxY) / 2) * scale })
  }
  return nodes
}

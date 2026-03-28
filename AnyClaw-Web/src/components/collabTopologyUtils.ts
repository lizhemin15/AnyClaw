export function canonPair(a: string, b: string): [string, string] {
  const x = a.trim()
  const y = b.trim()
  return x < y ? [x, y] : [y, x]
}

export function edgeKey(lo: string, hi: string): string {
  return `${lo}\0${hi}`
}

export function normalizeEdgesKey(list: [string, string][]): string {
  return [...list]
    .map(([a, b]) => edgeKey(...canonPair(a, b)))
    .sort()
    .join('|')
}

export function filterEdgesForSlugs(edges: [string, string][], slugs: Set<string>): [string, string][] {
  return edges.map(([x, y]) => canonPair(x, y)).filter(([lo, hi]) => slugs.has(lo) && slugs.has(hi))
}

export function canonPairNum(a: number, b: number): [number, number] {
  return a < b ? [a, b] : [b, a]
}

export function edgeKeyNum(lo: number, hi: number): string {
  return `${lo}\0${hi}`
}

export function normalizeEdgesKeyNum(list: [number, number][]): string {
  return [...list]
    .map(([a, b]) => edgeKeyNum(...canonPairNum(a, b)))
    .sort()
    .join('|')
}

export function filterEdgesForInstanceIds(
  edges: [number, number][],
  ids: Set<number>
): [number, number][] {
  return edges.map(([x, y]) => canonPairNum(x, y)).filter(([lo, hi]) => ids.has(lo) && ids.has(hi))
}

/** 节点中心坐标（viewBox 0–100） */
export function layoutAgents(n: number): { x: number; y: number }[] {
  if (n <= 0) return []
  if (n === 1) return [{ x: 50, y: 50 }]
  return Array.from({ length: n }, (_, i) => {
    const angle = (2 * Math.PI * i) / n - Math.PI / 2
    return { x: 50 + 36 * Math.cos(angle), y: 50 + 36 * Math.sin(angle) }
  })
}

export const DRAG_THRESHOLD_PX = 8

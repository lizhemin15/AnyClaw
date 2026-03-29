import type { CollabPeerInstance, Instance } from '../api'

/** JSON 解析后实例 id 可能为 number 或 string，统一为 number 以便 Set/边过滤一致 */
export function collabInstanceId(v: unknown): number | null {
  if (typeof v === 'number' && Number.isFinite(v)) return v
  if (typeof v === 'string' && v.trim() !== '') {
    const n = parseInt(v, 10)
    if (Number.isFinite(n)) return n
  }
  return null
}

/** 将 GET /collab/agents 的 peer_instances 转为与当前实例的无向边，并与 instance-topology 边合并去重（peer 即协作邻居） */
export function mergeInstanceTopologyEdgesWithPeers(
  selfId: number,
  edges: [unknown, unknown][],
  peers: CollabPeerInstance[] | undefined | null
): [number, number][] {
  const sid = collabInstanceId(selfId) ?? selfId
  const keys = new Set<string>()
  const out: [number, number][] = []
  const pushPair = (lo: number, hi: number) => {
    if (lo === hi) return
    const [a, b] = canonPairNum(lo, hi)
    const k = edgeKeyNum(a, b)
    if (keys.has(k)) return
    keys.add(k)
    out.push([a, b])
  }
  for (const row of edges || []) {
    const x = collabInstanceId(row[0])
    const y = collabInstanceId(row[1])
    if (x != null && y != null) pushPair(x, y)
  }
  if (peers?.length) {
    for (const p of peers) {
      const pid = collabInstanceId(p.instance_id)
      if (pid == null || pid === sid) continue
      pushPair(sid, pid)
    }
  }
  out.sort((a, b) => a[0] - b[0] || a[1] - b[1])
  return out
}

/** 将 GET /collab/agents 返回的 peer_instances 并入实例列表（补全编排拓扑中未出现在 GET /instances 的节点） */
export function mergeInstancesWithPeers(
  list: Instance[],
  peers: CollabPeerInstance[] | undefined | null
): Instance[] {
  if (!peers?.length) return list
  const byId = new Map<number, Instance>()
  for (const i of list) {
    const id = collabInstanceId(i.id)
    if (id != null) byId.set(id, { ...i, id })
  }
  for (const p of peers) {
    const id = collabInstanceId(p.instance_id)
    if (id == null) continue
    if (byId.has(id)) continue
    byId.set(id, {
      id,
      user_id: 0,
      name: (p.name && String(p.name).trim()) || `#${id}`,
      status: '',
      energy: 0,
      daily_consume: 0,
      created_at: '',
    })
  }
  return [...byId.values()].sort((a, b) => a.id - b.id)
}

/** 画布节点 id 与边端点比较时统一为 number（避免 JSON 中 string id 导致过滤掉全部边） */
export function instanceIdSetFromNodes(nodes: Instance[]): Set<number> {
  const s = new Set<number>()
  for (const n of nodes) {
    const id = collabInstanceId(n.id)
    if (id != null) s.add(id)
  }
  return s
}

/** 当前查看的实例若不在列表中则补一条（仅 id，名称占位） */
export function ensureInstanceInList(list: Instance[], instanceId: number): Instance[] {
  const want = collabInstanceId(instanceId) ?? instanceId
  if (list.some((x) => collabInstanceId(x.id) === want)) return list
  return [
    ...list,
    {
      id: instanceId,
      user_id: 0,
      name: `#${instanceId}`,
      status: '',
      energy: 0,
      daily_consume: 0,
      created_at: '',
    },
  ].sort((a, b) => a.id - b.id)
}

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
  edges: [unknown, unknown][],
  ids: Set<number>
): [number, number][] {
  const idOk = (n: number) => ids.has(n)
  return (edges || [])
    .map(([x, y]) => {
      const lo = collabInstanceId(x)
      const hi = collabInstanceId(y)
      if (lo == null || hi == null) return null
      return canonPairNum(lo, hi)
    })
    .filter((pair): pair is [number, number] => pair != null && pair[0] !== pair[1] && idOk(pair[0]) && idOk(pair[1]))
}

export type LayoutAgentsOptions = {
  /** 圆半径（viewBox 0–100），默认 36；窄屏可增大以拉开节点间距 */
  radius?: number
}

/** 节点中心坐标（viewBox 0–100） */
export function layoutAgents(n: number, opts?: LayoutAgentsOptions): { x: number; y: number }[] {
  const r = opts?.radius ?? 36
  if (n <= 0) return []
  if (n === 1) return [{ x: 50, y: 50 }]
  return Array.from({ length: n }, (_, i) => {
    const angle = (2 * Math.PI * i) / n - Math.PI / 2
    return { x: 50 + r * Math.cos(angle), y: 50 + r * Math.sin(angle) }
  })
}

/** FNV-1a 风格 32 位哈希，用于布局随机种子 */
export function hashString(s: string): number {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}

/** 画布世界坐标边长（节点与力导向布局使用同一空间） */
export const TOPOLOGY_WORLD_SIZE = 1000

export type ViewBoxRect = { x: number; y: number; w: number; h: number }

function mulberry32(seed: number) {
  return () => {
    let t = (seed += 0x6d2b79f5)
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

function dedupeSortedIndexPairs(pairs: [number, number][]): [number, number][] {
  pairs.sort((a, b) => a[0] - b[0] || a[1] - b[1])
  const dedup: [number, number][] = []
  let last = ''
  for (const [i, j] of pairs) {
    const k = `${i}\0${j}`
    if (k === last) continue
    last = k
    dedup.push([i, j])
  }
  return dedup
}

/** 将 slug 边转为节点下标边（用于力导向） */
export function indexEdgesFromSlugEdges(
  edges: [string, string][],
  slugToIndex: Map<string, number>
): [number, number][] {
  const out: [number, number][] = []
  for (const [a, b] of edges) {
    const i = slugToIndex.get(a.trim())
    const j = slugToIndex.get(b.trim())
    if (i == null || j == null || i === j) continue
    out.push(i < j ? [i, j] : [j, i])
  }
  return dedupeSortedIndexPairs(out)
}

/** 将实例 id 边转为节点下标边（用于力导向） */
export function indexEdgesFromInstanceEdges(
  edges: [number, number][],
  idToIndex: Map<number, number>
): [number, number][] {
  const out: [number, number][] = []
  for (const [a, b] of edges) {
    const i = idToIndex.get(a)
    const j = idToIndex.get(b)
    if (i == null || j == null || i === j) continue
    out.push(i < j ? [i, j] : [j, i])
  }
  return dedupeSortedIndexPairs(out)
}

/**
 * 力导向布局（Fruchterman–Reingold 风格），输出坐标在 [0, size]²。
 * 边列表为无向、端点为 0..n-1。
 */
export function layoutForceDirected(
  n: number,
  indexEdges: [number, number][],
  opts?: { size?: number; iterations?: number; seed?: number }
): { x: number; y: number }[] {
  const size = opts?.size ?? TOPOLOGY_WORLD_SIZE
  const area = size * size
  const k = Math.sqrt(area / Math.max(n, 1))
  const iterations =
    opts?.iterations ??
    Math.min(400, Math.max(60, Math.floor(140000 / Math.max(n, 1))))
  const rng = mulberry32(opts?.seed ?? 0x9e3779b9 ^ (n * 2654435761))

  if (n <= 0) return []
  if (n === 1) return [{ x: size / 2, y: size / 2 }]

  const x = new Float64Array(n)
  const y = new Float64Array(n)
  const ring = (size / 2) * 0.38
  const cx = size / 2
  const cy = size / 2
  for (let i = 0; i < n; i++) {
    const ang = (2 * Math.PI * i) / n - Math.PI / 2 + (rng() - 0.5) * 0.15
    const rad = ring * (0.85 + rng() * 0.3)
    x[i] = cx + rad * Math.cos(ang)
    y[i] = cy + rad * Math.sin(ang)
  }

  const t0 = 0.9
  for (let iter = 0; iter < iterations; iter++) {
    const t = t0 * (1 - iter / iterations)
    const dispX = new Float64Array(n)
    const dispY = new Float64Array(n)

    for (let i = 0; i < n; i++) {
      for (let j = i + 1; j < n; j++) {
        const dx = x[i] - x[j]
        const dy = y[i] - y[j]
        const dist = Math.hypot(dx, dy) || 0.01
        const rep = (k * k) / dist
        const rx = (rep * dx) / dist
        const ry = (rep * dy) / dist
        dispX[i] += rx
        dispY[i] += ry
        dispX[j] -= rx
        dispY[j] -= ry
      }
    }

    for (const [i, j] of indexEdges) {
      const dx = x[j] - x[i]
      const dy = y[j] - y[i]
      const dist = Math.hypot(dx, dy) || 0.01
      const att = ((dist * dist) / k) * 0.04
      const ax = (att * dx) / dist
      const ay = (att * dy) / dist
      dispX[i] += ax
      dispY[i] += ay
      dispX[j] -= ax
      dispY[j] -= ay
    }

    const g = 0.03 * t
    for (let i = 0; i < n; i++) {
      dispX[i] += (cx - x[i]) * g
      dispY[i] += (cy - y[i]) * g
    }

    for (let i = 0; i < n; i++) {
      const mag = Math.hypot(dispX[i], dispY[i]) || 0
      const cap = t * k
      const scale = mag > cap ? cap / mag : 1
      x[i] += dispX[i] * scale
      y[i] += dispY[i] * scale
      const pad = k * 0.35
      x[i] = Math.min(size - pad, Math.max(pad, x[i]))
      y[i] = Math.min(size - pad, Math.max(pad, y[i]))
    }
  }

  return Array.from({ length: n }, (_, i) => ({ x: x[i], y: y[i] }))
}

/** 根据节点位置计算初始 viewBox（含边距），使图整体可见 */
export function fitViewBoxToPositions(
  positions: { x: number; y: number }[],
  paddingRatio = 0.12,
  /** 世界坐标系下的节点半径，避免圆被裁切 */
  nodeRadiusWorld = 0
): ViewBoxRect {
  if (positions.length === 0) {
    return { x: 0, y: 0, w: TOPOLOGY_WORLD_SIZE, h: TOPOLOGY_WORLD_SIZE }
  }
  let minX = Infinity
  let minY = Infinity
  let maxX = -Infinity
  let maxY = -Infinity
  for (const p of positions) {
    minX = Math.min(minX, p.x)
    minY = Math.min(minY, p.y)
    maxX = Math.max(maxX, p.x)
    maxY = Math.max(maxY, p.y)
  }
  const nr = nodeRadiusWorld
  minX -= nr
  minY -= nr
  maxX += nr
  maxY += nr
  const padX = Math.max((maxX - minX) * paddingRatio, TOPOLOGY_WORLD_SIZE * 0.06)
  const padY = Math.max((maxY - minY) * paddingRatio, TOPOLOGY_WORLD_SIZE * 0.06)
  minX -= padX
  minY -= padY
  maxX += padX
  maxY += padY
  const w = Math.max(maxX - minX, 1)
  const h = Math.max(maxY - minY, 1)
  return { x: minX, y: minY, w, h }
}

/** 以某世界坐标为中心缩放 viewBox */
export function zoomViewBoxAt(vb: ViewBoxRect, worldX: number, worldY: number, factor: number): ViewBoxRect {
  const f = Math.min(Math.max(factor, 0.25), 4)
  const newW = vb.w * f
  const newH = vb.h * f
  return {
    x: worldX - (worldX - vb.x) * f,
    y: worldY - (worldY - vb.y) * f,
    w: newW,
    h: newH,
  }
}

export const DRAG_THRESHOLD_PX = 8

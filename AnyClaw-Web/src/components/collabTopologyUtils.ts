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

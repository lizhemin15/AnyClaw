import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  ANYCLAW_COLLAB_BROADCAST,
  broadcastCollabEvent,
  getCollabAgents,
  getCollabInstanceTopology,
  getCollabTopology,
  getInstances,
  putCollabInstanceTopology,
  putCollabTopology,
  type CollabAgent,
  type CollabApiError,
  type CollabLimits,
  type CollabPeerInstance,
  type Instance,
} from '../api'
import {
  DRAG_THRESHOLD_PX,
  TOPOLOGY_WORLD_SIZE,
  canonPair,
  canonPairNum,
  edgeKey,
  edgeKeyNum,
  filterEdgesForInstanceIds,
  filterEdgesForSlugs,
  fitViewBoxToPositions,
  hashString,
  indexEdgesFromInstanceEdges,
  indexEdgesFromSlugEdges,
  layoutForceDirected,
  ensureInstanceInList,
  instanceIdSetFromNodes,
  mergeInstanceTopologyEdgesWithPeers,
  mergeInstancesWithPeers,
  normalizeEdgesKey,
  normalizeEdgesKeyNum,
  zoomViewBoxAt,
  type ViewBoxRect,
} from './collabTopologyUtils'

/** 世界坐标系下半径，配合 fit 后约对应屏幕 10–15px 圆点 */
const NODE_RADIUS_WORLD = 14
const HIT_RADIUS_WORLD = 22

export type CollabTopologyPanelProps = {
  instanceId: number
  /** agents：实例内员工连线；instances：账号下全部实例节点与连线（GET /instances + instance-topology） */
  nodeSource?: 'agents' | 'instances'
  /** 协作名单保存后递增，以刷新节点列表 */
  rosterRevision?: number
  /** 拓扑保存成功（例如首页刷新实例列表） */
  onTopologySaved?: () => void
  /** 是否有未保存的连线变更（用于外层弹窗关闭确认） */
  onDirtyChange?: (dirty: boolean) => void
  className?: string
}

function findSlugUnderPoint(clientX: number, clientY: number): string | null {
  const stack = document.elementsFromPoint(clientX, clientY)
  for (const el of stack) {
    if (!(el instanceof HTMLElement)) continue
    const s = el.closest('[data-collab-node-slug]')?.getAttribute('data-collab-node-slug')
    if (s) return s
  }
  return null
}

export default function CollabTopologyPanel({
  instanceId,
  nodeSource = 'agents',
  rosterRevision = 0,
  onTopologySaved,
  onDirtyChange,
  className = '',
}: CollabTopologyPanelProps) {
  const isInstanceMode = nodeSource === 'instances'
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [loadWarn, setLoadWarn] = useState<string | null>(null)
  const [agents, setAgents] = useState<CollabAgent[]>([])
  const [peerInstances, setPeerInstances] = useState<CollabPeerInstance[]>([])
  const [instanceNodes, setInstanceNodes] = useState<Instance[]>([])
  const [limits, setLimits] = useState<CollabLimits | null>(null)
  const [edges, setEdges] = useState<[string, string][]>([])
  const [baselineEdges, setBaselineEdges] = useState<[string, string][]>([])
  const [instEdges, setInstEdges] = useState<[number, number][]>([])
  const [baselineInstEdges, setBaselineInstEdges] = useState<[number, number][]>([])
  const [pendingSlug, setPendingSlug] = useState<string | null>(null)
  const [topoVersion, setTopoVersion] = useState(0)
  const [topologyReady, setTopologyReady] = useState(false)
  const [staleRemote, setStaleRemote] = useState(false)

  const [dragFrom, setDragFrom] = useState<string | null>(null)
  const [dragCur, setDragCur] = useState<{ x: number; y: number } | null>(null)
  const dragMovedRef = useRef(false)
  const pressRef = useRef<{ slug: string; x: number; y: number } | null>(null)

  const panelRef = useRef<HTMLDivElement>(null)
  const fullscreenWrapRef = useRef<HTMLDivElement>(null)
  const svgRef = useRef<SVGSVGElement>(null)
  const [topologyFullscreen, setTopologyFullscreen] = useState(false)
  const panDraggingRef = useRef(false)
  const panLastRef = useRef<{ x: number; y: number } | null>(null)
  const instanceIdRef = useRef(instanceId)

  const [viewBox, setViewBox] = useState<ViewBoxRect>({
    x: 0,
    y: 0,
    w: TOPOLOGY_WORLD_SIZE,
    h: TOPOLOGY_WORLD_SIZE,
  })
  const [hoveredKey, setHoveredKey] = useState<string | null>(null)
  instanceIdRef.current = instanceId

  const mergeLimits = useCallback((a?: CollabLimits, b?: CollabLimits) => {
    if (b) setLimits(b)
    else if (a) setLimits(a)
  }, [])

  const load = useCallback(async () => {
    const expectedId = instanceId
    setLoading(true)
    setErr(null)
    setLoadWarn(null)
    setPendingSlug(null)
    setStaleRemote(false)
    setTopologyReady(false)
    setDragFrom(null)
    setDragCur(null)
    pressRef.current = null
    setLimits(null)
    setPeerInstances([])

    if (isInstanceMode) {
      const settled = await Promise.allSettled([
        getInstances(),
        getCollabInstanceTopology(expectedId),
        getCollabAgents(expectedId),
      ])
      if (instanceIdRef.current !== expectedId) {
        setLoading(false)
        return
      }
      const ir = settled[0]
      const tr = settled[1]
      const ar = settled[2]
      if (ir.status === 'rejected') {
        const r = ir.reason
        const msg = r instanceof Error ? r.message : String(r)
        if (r instanceof Error) {
          const lim = (r as CollabApiError).collabLimits
          if (lim) setLimits(lim)
        }
        setErr(msg)
        setInstanceNodes([])
        setPeerInstances([])
        setInstEdges([])
        setBaselineInstEdges([])
        setTopoVersion(0)
        setLoading(false)
        return
      }
      const peersFromRoster =
        ar.status === 'fulfilled' ? ar.value.peer_instances ?? [] : []
      if (ar.status === 'fulfilled') {
        mergeLimits(undefined, ar.value.limits)
        setPeerInstances(peersFromRoster)
      } else {
        setPeerInstances([])
      }
      const instList = ensureInstanceInList(
        mergeInstancesWithPeers(ir.value || [], peersFromRoster),
        expectedId
      )
      setInstanceNodes(instList)
      if (tr.status === 'rejected') {
        const r = tr.reason
        const msg = r instanceof Error ? r.message : String(r)
        if (r instanceof Error) {
          const lim = (r as CollabApiError).collabLimits
          if (lim) setLimits(lim)
        }
        setLoadWarn(`拓扑未加载：${msg}。请「重新加载」后再保存，以免覆盖服务端拓扑。`)
        setInstEdges([])
        setBaselineInstEdges([])
        setTopoVersion(0)
        setTopologyReady(false)
        setLoading(false)
        return
      }
      const t = tr.value
      mergeLimits(undefined, t.limits)
      const idSet = instanceIdSetFromNodes(instList)
      const raw = (t.edges || []) as [unknown, unknown][]
      const mapped = filterEdgesForInstanceIds(raw, idSet)
      const merged = mergeInstanceTopologyEdgesWithPeers(expectedId, mapped, peersFromRoster)
      setTopoVersion(typeof t.version === 'number' ? t.version : 0)
      setBaselineInstEdges(merged)
      setInstEdges(merged)
      setTopologyReady(true)
      setLoading(false)
      return
    }

    const settled = await Promise.allSettled([getCollabAgents(expectedId), getCollabTopology(expectedId)])

    if (instanceIdRef.current !== expectedId) {
      setLoading(false)
      return
    }

    const ar = settled[0]
    const tr = settled[1]

    if (ar.status === 'rejected') {
      const r = ar.reason
      const msg = r instanceof Error ? r.message : String(r)
      if (r instanceof Error) {
        const lim = (r as CollabApiError).collabLimits
        if (lim) setLimits(lim)
      }
      setErr(msg)
      setAgents([])
      setPeerInstances([])
      setEdges([])
      setBaselineEdges([])
      setTopoVersion(0)
      setLoading(false)
      return
    }

    const a = ar.value
    const list = a.agents || []
    setAgents(list)
    setPeerInstances(a.peer_instances ?? [])
    mergeLimits(undefined, a.limits)

    if (tr.status === 'rejected') {
      const r = tr.reason
      const msg = r instanceof Error ? r.message : String(r)
      if (r instanceof Error) {
        const lim = (r as CollabApiError).collabLimits
        if (lim) setLimits(lim)
      }
      setLoadWarn(`拓扑未加载：${msg}。请「重新加载」后再保存，以免覆盖服务端拓扑。`)
      setEdges([])
      setBaselineEdges([])
      setTopoVersion(0)
      setTopologyReady(false)
      setLoading(false)
      return
    }

    const t = tr.value
    mergeLimits(a.limits, t.limits)
    const slugSet = new Set(list.map((x) => x.agent_slug.trim()))
    const mapped = filterEdgesForSlugs(t.edges || [], slugSet)
    setTopoVersion(typeof t.version === 'number' ? t.version : 0)
    setBaselineEdges(mapped)
    setEdges(mapped)
    setTopologyReady(true)
    setLoading(false)
  }, [instanceId, mergeLimits, isInstanceMode])

  useEffect(() => {
    void load()
  }, [load, rosterRevision])

  const slugSet = useMemo(() => new Set(agents.map((a) => a.agent_slug.trim())), [agents])
  const instanceIdSet = useMemo(() => instanceIdSetFromNodes(instanceNodes), [instanceNodes])

  const dirty = useMemo(() => {
    if (!topologyReady) return false
    if (isInstanceMode) {
      if (instanceNodes.length < 2) return false
      return normalizeEdgesKeyNum(instEdges) !== normalizeEdgesKeyNum(baselineInstEdges)
    }
    if (agents.length < 2) return false
    return normalizeEdgesKey(edges) !== normalizeEdgesKey(baselineEdges)
  }, [
    isInstanceMode,
    topologyReady,
    instanceNodes.length,
    instEdges,
    baselineInstEdges,
    agents.length,
    edges,
    baselineEdges,
  ])

  useEffect(() => {
    onDirtyChange?.(dirty)
  }, [dirty, onDirtyChange])

  useEffect(() => {
    const fullscreenEl = (): Element | null =>
      document.fullscreenElement ??
      (document as Document & { webkitFullscreenElement?: Element | null }).webkitFullscreenElement ??
      null
    const onFullscreenChange = () => {
      const wrap = fullscreenWrapRef.current
      const active = wrap != null && fullscreenEl() === wrap
      setTopologyFullscreen(active)
      if (!active) {
        try {
          const o = screen.orientation as ScreenOrientation & { unlock?: () => void }
          o?.unlock?.()
        } catch {
          /* noop */
        }
      }
    }
    document.addEventListener('fullscreenchange', onFullscreenChange)
    document.addEventListener('webkitfullscreenchange', onFullscreenChange as EventListener)
    return () => {
      document.removeEventListener('fullscreenchange', onFullscreenChange)
      document.removeEventListener('webkitfullscreenchange', onFullscreenChange as EventListener)
    }
  }, [])

  const enterTopologyFullscreen = useCallback(async () => {
    const el = fullscreenWrapRef.current
    if (!el) return
    const req =
      el.requestFullscreen?.bind(el) ??
      (el as HTMLElement & { webkitRequestFullscreen?: () => Promise<void> }).webkitRequestFullscreen?.bind(el)
    if (!req) return
    try {
      await req()
      try {
        await screen.orientation?.lock?.('landscape')
      } catch {
        /* 部分环境不支持或需已全屏 */
      }
    } catch {
      /* 用户拒绝或浏览器不支持 */
    }
  }, [])

  const exitTopologyFullscreen = useCallback(async () => {
    const exit =
      document.exitFullscreen?.bind(document) ??
      (document as Document & { webkitExitFullscreen?: () => Promise<void> }).webkitExitFullscreen?.bind(document)
    if (!exit) return
    try {
      await exit()
    } catch {
      /* noop */
    }
  }, [])

  const dirtyRef = useRef(false)
  dirtyRef.current = dirty

  useEffect(() => {
    if (typeof BroadcastChannel === 'undefined') return
    let bc: BroadcastChannel | null = null
    try {
      bc = new BroadcastChannel(ANYCLAW_COLLAB_BROADCAST)
      bc.onmessage = (ev: MessageEvent) => {
        const d = ev.data as { kind?: string; instanceId?: number }
        if (isInstanceMode) {
          if (d.kind !== 'user_instance_topology') return
          if (dirtyRef.current) {
            setStaleRemote(true)
            return
          }
          void load()
          return
        }
        if (d.kind === 'user_instance_topology') return
        if (d.instanceId !== instanceId) return
        if (d.kind !== 'topology') return
        if (dirtyRef.current) {
          setStaleRemote(true)
          return
        }
        void load()
      }
    } catch {
      /* noop */
    }
    return () => bc?.close()
  }, [instanceId, load, isInstanceMode])

  const maxEdges = limits?.max_edges ?? 4096
  const slugToIndex = useMemo(() => {
    const m = new Map<string, number>()
    agents.forEach((a, i) => m.set(a.agent_slug.trim(), i))
    return m
  }, [agents])
  const instIdToIndex = useMemo(() => {
    const m = new Map<number, number>()
    instanceNodes.forEach((a, i) => m.set(a.id, i))
    return m
  }, [instanceNodes])

  const edgeSet = useMemo(() => {
    const s = new Set<string>()
    for (const [lo, hi] of edges) s.add(edgeKey(lo, hi))
    return s
  }, [edges])

  const edgeSetInst = useMemo(() => {
    const s = new Set<string>()
    for (const [lo, hi] of instEdges) s.add(edgeKeyNum(lo, hi))
    return s
  }, [instEdges])

  const displayEdges = useMemo(
    () => edges.filter(([lo, hi]) => slugSet.has(lo) && slugSet.has(hi)),
    [edges, slugSet]
  )

  const displayInstEdges = useMemo(
    () => instEdges.filter(([lo, hi]) => instanceIdSet.has(lo) && instanceIdSet.has(hi)),
    [instEdges, instanceIdSet]
  )

  const layoutResetKey = useMemo(() => {
    if (isInstanceMode) {
      const ids = [...instanceNodes]
        .map((n) => n.id)
        .sort((a, b) => Number(a) - Number(b))
        .join(',')
      return `${instanceNodes.length}|${normalizeEdgesKeyNum(instEdges)}|${ids}`
    }
    const slugs = agents.map((a) => a.agent_slug.trim()).sort().join(',')
    return `${agents.length}|${normalizeEdgesKey(edges)}|${slugs}`
  }, [isInstanceMode, instanceNodes, agents, instEdges, edges])

  const worldPos = useMemo(() => {
    const n = isInstanceMode ? instanceNodes.length : agents.length
    if (n === 0) return []
    const indexEdges = isInstanceMode
      ? indexEdgesFromInstanceEdges(displayInstEdges, instIdToIndex)
      : indexEdgesFromSlugEdges(displayEdges, slugToIndex)
    const seed = hashString(layoutResetKey)
    return layoutForceDirected(n, indexEdges, { seed })
  }, [
    isInstanceMode,
    instanceNodes.length,
    agents.length,
    displayInstEdges,
    displayEdges,
    slugToIndex,
    instIdToIndex,
    layoutResetKey,
  ])

  useEffect(() => {
    if (!topologyReady || worldPos.length === 0) return
    setViewBox(fitViewBoxToPositions(worldPos, 0.12, NODE_RADIUS_WORLD))
  }, [topologyReady, layoutResetKey, worldPos])

  const clientToSvg = useCallback((clientX: number, clientY: number): { x: number; y: number } | null => {
    const svg = svgRef.current
    if (!svg) return null
    const pt = svg.createSVGPoint()
    pt.x = clientX
    pt.y = clientY
    const ctm = svg.getScreenCTM()
    if (!ctm) return null
    const loc = pt.matrixTransform(ctm.inverse())
    return { x: loc.x, y: loc.y }
  }, [])

  useEffect(() => {
    if (loading) return
    const el = svgRef.current
    if (!el) return
    const onWheel = (e: WheelEvent) => {
      e.preventDefault()
      const svg = svgRef.current
      if (!svg) return
      const pt = svg.createSVGPoint()
      pt.x = e.clientX
      pt.y = e.clientY
      const ctm = svg.getScreenCTM()
      if (!ctm) return
      const loc = pt.matrixTransform(ctm.inverse())
      const factor = e.deltaY > 0 ? 1.08 : 0.92
      setViewBox((vb) => {
        const next = zoomViewBoxAt(vb, loc.x, loc.y, factor)
        const minW = TOPOLOGY_WORLD_SIZE * 0.02
        const maxW = TOPOLOGY_WORLD_SIZE * 2.5
        if (next.w < minW || next.w > maxW) return vb
        return next
      })
    }
    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  }, [loading, topologyReady, layoutResetKey])

  const handleFitView = useCallback(() => {
    if (worldPos.length === 0) return
    setViewBox(fitViewBoxToPositions(worldPos, 0.12, NODE_RADIUS_WORLD))
  }, [worldPos])

  const zoomAtCenter = useCallback((factor: number) => {
    setViewBox((vb) => {
      const cx = vb.x + vb.w / 2
      const cy = vb.y + vb.h / 2
      const next = zoomViewBoxAt(vb, cx, cy, factor)
      const minW = TOPOLOGY_WORLD_SIZE * 0.02
      const maxW = TOPOLOGY_WORLD_SIZE * 2.5
      if (next.w < minW || next.w > maxW) return vb
      return next
    })
  }, [])

  const handlePanPointerDown = (e: React.PointerEvent) => {
    if (e.button !== 0) return
    e.preventDefault()
    panDraggingRef.current = true
    panLastRef.current = { x: e.clientX, y: e.clientY }
    try {
      ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
    } catch {
      /* noop */
    }
  }

  const handlePanPointerMove = (e: React.PointerEvent) => {
    if (!panDraggingRef.current || !panLastRef.current) return
    const svg = svgRef.current
    if (!svg) return
    const rect = svg.getBoundingClientRect()
    const dx = e.clientX - panLastRef.current.x
    const dy = e.clientY - panLastRef.current.y
    panLastRef.current = { x: e.clientX, y: e.clientY }
    setViewBox((vb) => ({
      x: vb.x - (dx / rect.width) * vb.w,
      y: vb.y - (dy / rect.height) * vb.h,
      w: vb.w,
      h: vb.h,
    }))
  }

  const handlePanPointerUp = (e: React.PointerEvent) => {
    if (panDraggingRef.current) {
      try {
        ;(e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId)
      } catch {
        /* noop */
      }
    }
    panDraggingRef.current = false
    panLastRef.current = null
  }

  const handleMinimapClick = (e: React.MouseEvent<SVGSVGElement>) => {
    const svg = e.currentTarget
    const pt = svg.createSVGPoint()
    pt.x = e.clientX
    pt.y = e.clientY
    const ctm = svg.getScreenCTM()
    if (!ctm) return
    const loc = pt.matrixTransform(ctm.inverse())
    setViewBox((vb) => {
      let nx = loc.x - vb.w / 2
      let ny = loc.y - vb.h / 2
      const maxX = Math.max(0, TOPOLOGY_WORLD_SIZE - vb.w)
      const maxY = Math.max(0, TOPOLOGY_WORLD_SIZE - vb.h)
      nx = Math.max(0, Math.min(maxX, nx))
      ny = Math.max(0, Math.min(maxY, ny))
      return { x: nx, y: ny, w: vb.w, h: vb.h }
    })
  }

  const toggleEdge = useCallback(
    (slugA: string, slugB: string) => {
      const [lo, hi] = canonPair(slugA, slugB)
      if (lo === hi) return
      const k = edgeKey(lo, hi)
      if (edgeSet.has(k)) {
        setEdges((prev) => prev.filter(([x, y]) => edgeKey(x, y) !== k))
        return
      }
      if (edges.length >= maxEdges) {
        setErr(`连线已达上限（${maxEdges} 条）`)
        return
      }
      setEdges((prev) => [...prev, [lo, hi]])
      setErr(null)
      setStaleRemote(false)
    },
    [edgeSet, edges.length, maxEdges]
  )

  const toggleInstEdge = useCallback(
    (idA: number, idB: number) => {
      const [lo, hi] = canonPairNum(idA, idB)
      if (lo === hi) return
      const k = edgeKeyNum(lo, hi)
      if (edgeSetInst.has(k)) {
        setInstEdges((prev) =>
          prev.filter(([x, y]) => edgeKeyNum(...canonPairNum(x, y)) !== k)
        )
        return
      }
      if (instEdges.length >= maxEdges) {
        setErr(`连线已达上限（${maxEdges} 条）`)
        return
      }
      setInstEdges((prev) => [...prev, [lo, hi]])
      setErr(null)
      setStaleRemote(false)
    },
    [edgeSetInst, instEdges.length, maxEdges]
  )

  const toggleEdgeOrInst = useCallback(
    (keyA: string, keyB: string) => {
      if (isInstanceMode) {
        const a = parseInt(keyA, 10)
        const b = parseInt(keyB, 10)
        if (!Number.isFinite(a) || !Number.isFinite(b)) return
        toggleInstEdge(a, b)
      } else {
        toggleEdge(keyA, keyB)
      }
    },
    [isInstanceMode, toggleEdge, toggleInstEdge]
  )

  const onNodeClickSelect = useCallback(
    (slug: string) => {
      if (!topologyReady) return
      if (!pendingSlug) {
        setPendingSlug(slug)
        return
      }
      if (pendingSlug === slug) {
        setPendingSlug(null)
        return
      }
      toggleEdgeOrInst(pendingSlug, slug)
      setPendingSlug(null)
    },
    [topologyReady, pendingSlug, toggleEdgeOrInst]
  )

  const interactionRef = useRef({
    topologyReady,
    nodeCount: isInstanceMode ? instanceNodes.length : agents.length,
    toggleEdgeOrInst,
    onNodeClickSelect,
  })
  interactionRef.current = {
    topologyReady,
    nodeCount: isInstanceMode ? instanceNodes.length : agents.length,
    toggleEdgeOrInst,
    onNodeClickSelect,
  }

  const onNodePointerDown = (slug: string, e: React.PointerEvent) => {
    if (e.button !== 0) return
    const { topologyReady: tr, nodeCount } = interactionRef.current
    if (!tr || nodeCount < 2) return
    e.preventDefault()
    e.stopPropagation()
    pressRef.current = { slug, x: e.clientX, y: e.clientY }
    dragMovedRef.current = false

    const onMove = (ev: PointerEvent) => {
      const p = pressRef.current
      if (!p) return
      const dx = ev.clientX - p.x
      const dy = ev.clientY - p.y
      if (!dragMovedRef.current && dx * dx + dy * dy >= DRAG_THRESHOLD_PX * DRAG_THRESHOLD_PX) {
        dragMovedRef.current = true
        setDragFrom(p.slug)
      }
      if (dragMovedRef.current) {
        const svg = clientToSvg(ev.clientX, ev.clientY)
        if (svg) setDragCur(svg)
      }
    }

    const onUp = (ev: PointerEvent) => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('pointercancel', onUp)

      const p = pressRef.current
      pressRef.current = null
      const moved = dragMovedRef.current
      dragMovedRef.current = false
      setDragFrom(null)
      setDragCur(null)

      const { topologyReady: ok, nodeCount: n, toggleEdgeOrInst: te, onNodeClickSelect: clickSel } =
        interactionRef.current
      if (!p || !ok || n < 2) return

      const targetSlug = findSlugUnderPoint(ev.clientX, ev.clientY)
      if (moved) {
        if (targetSlug && targetSlug !== p.slug) te(p.slug, targetSlug)
        return
      }
      if (targetSlug === p.slug) {
        clickSel(p.slug)
      } else if (targetSlug && targetSlug !== p.slug) {
        te(p.slug, targetSlug)
      }
    }

    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    window.addEventListener('pointercancel', onUp)
  }

  const handleRestore = () => {
    if (isInstanceMode) {
      setInstEdges([...baselineInstEdges])
    } else {
      setEdges([...baselineEdges])
    }
    setPendingSlug(null)
    setErr(null)
    setStaleRemote(false)
  }

  const handleSave = async () => {
    if (!topologyReady) return
    if (isInstanceMode) {
      if (instanceNodes.length < 2) return
      const cleaned = filterEdgesForInstanceIds(instEdges, instanceIdSet)
      setSaving(true)
      setErr(null)
      try {
        await putCollabInstanceTopology(instanceId, cleaned)
        setBaselineInstEdges(cleaned)
        setInstEdges(cleaned)
        broadcastCollabEvent('user_instance_topology', instanceId)
        onTopologySaved?.()
      } catch (e) {
        const lim = (e as CollabApiError).collabLimits
        if (lim) setLimits(lim)
        setErr(e instanceof Error ? e.message : String(e))
      } finally {
        setSaving(false)
      }
      return
    }
    if (agents.length < 2) return
    const cleaned = filterEdgesForSlugs(edges, slugSet)
    setSaving(true)
    setErr(null)
    try {
      await putCollabTopology(instanceId, cleaned)
      setBaselineEdges(cleaned)
      setEdges(cleaned)
      broadcastCollabEvent('topology', instanceId)
      onTopologySaved?.()
    } catch (e) {
      const lim = (e as CollabApiError).collabLimits
      if (lim) setLimits(lim)
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  const nodeCountOk = isInstanceMode
    ? instanceNodes.length > 1
    : agents.length > 1 || peerInstances.length > 0
  const canSave = topologyReady && nodeCountOk && dirty && !saving && !loading
  /** agent 模式：同实例 slug 连线 + 账号编排 peer_instances；实例模式边已在 load 时与 peer 合并 */
  const edgeCount = isInstanceMode ? instEdges.length : edges.length + peerInstances.length
  const loadFailedEmpty = err && (isInstanceMode ? instanceNodes.length === 0 : agents.length === 0)
  const noNodes = isInstanceMode ? instanceNodes.length === 0 : agents.length === 0

  return (
    <div className={`rounded-2xl border border-violet-200/80 bg-gradient-to-b from-violet-50/40 to-white p-4 sm:p-5 space-y-3 ${className}`}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-semibold text-slate-800">通讯拓扑</h3>
        {nodeCountOk && !loading && topologyReady && (
          <span className="text-xs tabular-nums text-slate-500 bg-white border border-slate-200 px-2 py-0.5 rounded-md">
            连线 {edgeCount}/{maxEdges}
            {topoVersion > 0 ? ` · v${topoVersion}` : ''}
            {dirty ? ' · 未保存' : ''}
          </span>
        )}
      </div>
      <p className="text-xs text-slate-500 leading-relaxed">
        {isInstanceMode ? (
          <>
            画布上为账号下招募实例（<code className="bg-slate-100 px-0.5 rounded text-[11px]">GET /instances</code>
            ）并与 <code className="bg-slate-100 px-0.5 rounded text-[11px]">GET /collab/agents</code> 的{' '}
            <code className="bg-slate-100 px-0.5 rounded text-[11px]">peer_instances</code>（与{' '}
            <code className="bg-slate-100 px-0.5 rounded text-[11px]">…/collab/bridge/roster</code> 同源）合并。可<strong>拖拽</strong>连线或依次<strong>点击</strong>两节点。
          </>
        ) : (
          <>
            画布上为当前实例全部协作成员（打开时由 API 按已同步的{' '}
            <code className="bg-slate-100 px-0.5 rounded text-[11px]">agents.list</code> 自动补全）。可<strong>拖拽</strong>从一节点拉到另一节点以添加或移除连线（无向）；也可依次<strong>点击</strong>两个节点。展示名可在「协作展示名」页修改。
            {peerInstances.length > 0 && (
              <span className="block mt-1.5 text-slate-600">
                编排邻居实例：{peerInstances.map((p) => `${p.name || `#${p.instance_id}`} (#${p.instance_id})`).join('、')}
              </span>
            )}
          </>
        )}
      </p>

      {staleRemote && (
        <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900 flex flex-wrap items-center justify-between gap-2">
          <span>其它标签页已保存协作拓扑，与当前编辑不一致。</span>
          <button
            type="button"
            className="shrink-0 px-2 py-1 rounded-md bg-white border border-amber-300 text-amber-900 hover:bg-amber-100"
            onClick={() => void load()}
          >
            重新加载（放弃未保存）
          </button>
        </div>
      )}
      {loadWarn && !err && (
        <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900">{loadWarn}</div>
      )}

      {loading ? (
        <p className="text-slate-500 text-sm py-10 text-center">加载拓扑…</p>
      ) : loadFailedEmpty ? (
        <div className="py-6 text-center space-y-3">
          <p className="text-red-600 text-sm">{err}</p>
          <button
            type="button"
            onClick={() => void load()}
            className="text-sm text-indigo-600 px-3 py-1.5 border border-indigo-200 rounded-lg hover:bg-indigo-50"
          >
            重新加载
          </button>
        </div>
      ) : noNodes ? (
        <p className="text-slate-500 text-sm py-8 text-center">{isInstanceMode ? '暂无实例' : '暂无协作成员数据'}</p>
      ) : (
        <>
          {!isInstanceMode && agents.length === 1 && peerInstances.length === 0 && (
            <p className="text-xs text-amber-800 bg-amber-50 border border-amber-100 rounded-lg px-3 py-2">
              当前仅一名协作成员。多智能体时请先让容器启动并完成与 API 的协作同步；若实例已绑定宿主机，打开本页时会从工作区 `config.json` 的 `agents.list` 自动补全。也可在「协作展示名」页手动添加员工并保存后再连线。
            </p>
          )}
          {isInstanceMode && instanceNodes.length === 1 && (
            <p className="text-xs text-amber-800 bg-amber-50 border border-amber-100 rounded-lg px-3 py-2">
              当前仅一名招募实例。再招聘一名员工后即可连线。
            </p>
          )}
          <div className="flex flex-wrap items-center gap-2 min-h-[1.5rem]">
            {pendingSlug && topologyReady && nodeCountOk && (
              <>
                <p className="text-xs text-indigo-600 flex-1 min-w-[12rem]">
                  已选「{pendingSlug}」，再点另一节点连线或移除边
                </p>
                <button
                  type="button"
                  onClick={() => setPendingSlug(null)}
                  className="text-xs px-2 py-1 text-slate-600 border border-slate-200 rounded-lg hover:bg-slate-50"
                >
                  取消选择
                </button>
              </>
            )}
          </div>
          {dirty && topologyReady && nodeCountOk && (
            <button
              type="button"
              onClick={handleRestore}
              className="text-xs text-slate-600 px-2 py-1 border border-dashed border-slate-300 rounded-lg hover:bg-slate-50"
            >
              还原为上次加载
            </button>
          )}

          <div
            ref={(el) => {
              panelRef.current = el
              fullscreenWrapRef.current = el
            }}
            title="滚轮缩放；拖拽空白处平移；点击圆点或拖拽连线"
            className="relative mx-auto w-full min-h-0 min-w-0 max-w-[min(100%,min(85vh,36rem))] aspect-[4/3] max-h-[min(85vh,36rem)] overflow-hidden select-none touch-none rounded-xl border border-slate-200/80 bg-[#f8fafc] fullscreen:mx-0 fullscreen:max-h-none fullscreen:max-w-none fullscreen:aspect-auto fullscreen:h-full fullscreen:min-h-[100dvh] fullscreen:w-full fullscreen:rounded-none fullscreen:border-0"
          >
            {!topologyFullscreen && (
              <button
                type="button"
                onClick={() => void enterTopologyFullscreen()}
                className="absolute top-2 left-2 z-20 min-h-[44px] min-w-[44px] rounded-lg border border-indigo-200 bg-indigo-600 px-3 py-2 text-sm font-medium text-white shadow-md hover:bg-indigo-700 active:bg-indigo-800 touch-manipulation"
              >
                全屏
              </button>
            )}
            {topologyFullscreen && (
              <button
                type="button"
                onClick={() => void exitTopologyFullscreen()}
                className="absolute top-2 left-2 z-20 min-h-[48px] rounded-lg border border-slate-300 bg-white px-4 py-2.5 text-sm font-semibold text-slate-800 shadow-lg hover:bg-slate-50 active:bg-slate-100 touch-manipulation"
              >
                退出全屏
              </button>
            )}
            <div className="absolute top-2 right-2 z-10 flex flex-col gap-0.5 rounded-md border border-slate-200/90 bg-white/95 p-0.5 shadow-sm">
              <button
                type="button"
                aria-label="放大"
                onClick={() => zoomAtCenter(0.92)}
                className="min-h-[32px] min-w-[32px] rounded text-lg leading-none text-slate-700 hover:bg-slate-100"
              >
                +
              </button>
              <button
                type="button"
                aria-label="缩小"
                onClick={() => zoomAtCenter(1.08)}
                className="min-h-[32px] min-w-[32px] rounded text-lg leading-none text-slate-700 hover:bg-slate-100"
              >
                −
              </button>
              <button
                type="button"
                aria-label="适应画布"
                onClick={handleFitView}
                className="min-h-[32px] min-w-[32px] rounded text-xs font-medium text-slate-700 hover:bg-slate-100"
              >
                适应
              </button>
            </div>
            <div className="absolute bottom-2 right-2 z-10 rounded border border-slate-200/90 bg-white/95 p-0.5 shadow-sm">
              <svg
                viewBox={`0 0 ${TOPOLOGY_WORLD_SIZE} ${TOPOLOGY_WORLD_SIZE}`}
                preserveAspectRatio="xMidYMid meet"
                className="block h-[72px] w-[96px] sm:h-[88px] sm:w-[112px] cursor-pointer touch-manipulation"
                onClick={handleMinimapClick}
                role="img"
                aria-label="缩略图：点击跳转视野"
              >
                <rect width={TOPOLOGY_WORLD_SIZE} height={TOPOLOGY_WORLD_SIZE} fill="#f1f5f9" />
                {worldPos.map((p, i) => (
                  <circle key={i} cx={p.x} cy={p.y} r={3} fill="#cbd5e1" />
                ))}
                <rect
                  x={viewBox.x}
                  y={viewBox.y}
                  width={viewBox.w}
                  height={viewBox.h}
                  fill="none"
                  stroke="#6366f1"
                  strokeWidth={2}
                  vectorEffect="nonScalingStroke"
                  pointerEvents="none"
                />
              </svg>
            </div>
            <svg
              ref={svgRef}
              viewBox={`${viewBox.x} ${viewBox.y} ${viewBox.w} ${viewBox.h}`}
              preserveAspectRatio="xMidYMid meet"
              className="absolute inset-0 block h-full w-full touch-manipulation"
              aria-hidden
            >
              <rect
                x={viewBox.x}
                y={viewBox.y}
                width={viewBox.w}
                height={viewBox.h}
                fill="transparent"
                className="cursor-grab active:cursor-grabbing"
                onPointerDown={handlePanPointerDown}
                onPointerMove={handlePanPointerMove}
                onPointerUp={handlePanPointerUp}
                onPointerCancel={handlePanPointerUp}
              />
              {topologyReady &&
                (isInstanceMode
                  ? displayInstEdges.map(([lo, hi]) => {
                      const i = instIdToIndex.get(lo)
                      const j = instIdToIndex.get(hi)
                      if (i == null || j == null) return null
                      const p = worldPos[i]
                      const q = worldPos[j]
                      if (!p || !q) return null
                      const ka = String(lo)
                      const kb = String(hi)
                      const hiLn =
                        (pendingSlug && (pendingSlug === ka || pendingSlug === kb)) ||
                        (hoveredKey && (hoveredKey === ka || hoveredKey === kb))
                      return (
                        <line
                          key={`${lo}-${hi}`}
                          x1={p.x}
                          y1={p.y}
                          x2={q.x}
                          y2={q.y}
                          stroke={hiLn ? '#6366f1' : '#94a3b8'}
                          strokeOpacity={hiLn ? 0.95 : 0.45}
                          strokeWidth={1}
                          strokeLinecap="round"
                          vectorEffect="nonScalingStroke"
                          pointerEvents="none"
                        />
                      )
                    })
                  : displayEdges.map(([lo, hi]) => {
                      const i = slugToIndex.get(lo)
                      const j = slugToIndex.get(hi)
                      if (i == null || j == null) return null
                      const p = worldPos[i]
                      const q = worldPos[j]
                      if (!p || !q) return null
                      const hiLn =
                        (pendingSlug && (pendingSlug === lo || pendingSlug === hi)) ||
                        (hoveredKey && (hoveredKey === lo || hoveredKey === hi))
                      return (
                        <line
                          key={`${lo}-${hi}`}
                          x1={p.x}
                          y1={p.y}
                          x2={q.x}
                          y2={q.y}
                          stroke={hiLn ? '#6366f1' : '#94a3b8'}
                          strokeOpacity={hiLn ? 0.95 : 0.45}
                          strokeWidth={1}
                          strokeLinecap="round"
                          vectorEffect="nonScalingStroke"
                          pointerEvents="none"
                        />
                      )
                    }))}
              {dragFrom &&
                dragCur &&
                (() => {
                  if (isInstanceMode) {
                    const id = parseInt(dragFrom, 10)
                    const i = instIdToIndex.get(id)
                    if (i == null) return null
                    const p = worldPos[i]
                    if (!p) return null
                    return (
                      <line
                        x1={p.x}
                        y1={p.y}
                        x2={dragCur.x}
                        y2={dragCur.y}
                        stroke="#a5b4fc"
                        strokeWidth={1}
                        strokeDasharray="4 3"
                        strokeLinecap="round"
                        vectorEffect="nonScalingStroke"
                        pointerEvents="none"
                      />
                    )
                  }
                  const i = slugToIndex.get(dragFrom)
                  if (i == null) return null
                  const p = worldPos[i]
                  if (!p) return null
                  return (
                    <line
                      x1={p.x}
                      y1={p.y}
                      x2={dragCur.x}
                      y2={dragCur.y}
                      stroke="#a5b4fc"
                      strokeWidth={1}
                      strokeDasharray="4 3"
                      strokeLinecap="round"
                      vectorEffect="nonScalingStroke"
                      pointerEvents="none"
                    />
                  )
                })()}
              {isInstanceMode
                ? instanceNodes.map((inst, i) => {
                    const p = worldPos[i]
                    if (!p) return null
                    const key = String(inst.id)
                    const selected = pendingSlug === key
                    const nodeBusy = dragFrom === key
                    const label = `${inst.name} (#${inst.id})`
                    return (
                      <g key={inst.id} data-collab-node-slug={key}>
                        <circle
                          cx={p.x}
                          cy={p.y}
                          r={HIT_RADIUS_WORLD}
                          fill="transparent"
                          className={topologyReady ? 'cursor-pointer' : 'cursor-not-allowed'}
                          aria-hidden
                          pointerEvents={topologyReady ? 'auto' : 'none'}
                          onPointerEnter={() => topologyReady && setHoveredKey(key)}
                          onPointerLeave={() => setHoveredKey(null)}
                          onPointerDown={(e) => onNodePointerDown(key, e)}
                        />
                        <circle
                          cx={p.x}
                          cy={p.y}
                          r={NODE_RADIUS_WORLD}
                          fill={selected ? '#4f46e5' : nodeBusy ? '#a5b4fc' : '#e2e8f0'}
                          stroke={selected ? '#312e81' : '#94a3b8'}
                          strokeWidth={selected ? 2 : 1}
                          vectorEffect="nonScalingStroke"
                          pointerEvents="none"
                        />
                        <title>{label}</title>
                      </g>
                    )
                  })
                : agents.map((a, i) => {
                    const p = worldPos[i]
                    if (!p) return null
                    const slug = a.agent_slug.trim()
                    const selected = pendingSlug === slug
                    const nodeBusy = dragFrom === slug
                    const label = `${a.display_name} (${slug})`
                    return (
                      <g key={`${a.id}-${slug}`} data-collab-node-slug={slug}>
                        <circle
                          cx={p.x}
                          cy={p.y}
                          r={HIT_RADIUS_WORLD}
                          fill="transparent"
                          className={topologyReady ? 'cursor-pointer' : 'cursor-not-allowed'}
                          aria-hidden
                          pointerEvents={topologyReady ? 'auto' : 'none'}
                          onPointerEnter={() => topologyReady && setHoveredKey(slug)}
                          onPointerLeave={() => setHoveredKey(null)}
                          onPointerDown={(e) => onNodePointerDown(slug, e)}
                        />
                        <circle
                          cx={p.x}
                          cy={p.y}
                          r={NODE_RADIUS_WORLD}
                          fill={selected ? '#4f46e5' : nodeBusy ? '#a5b4fc' : '#e2e8f0'}
                          stroke={selected ? '#312e81' : '#94a3b8'}
                          strokeWidth={selected ? 2 : 1}
                          vectorEffect="nonScalingStroke"
                          pointerEvents="none"
                        />
                        <title>{label}</title>
                      </g>
                    )
                  })}
            </svg>
          </div>

          <div className="flex flex-wrap items-center justify-end gap-2 pt-1">
            <button
              type="button"
              disabled={!canSave}
              onClick={() => void handleSave()}
              className="px-4 py-2 text-sm bg-slate-800 text-white rounded-xl disabled:opacity-50 disabled:cursor-not-allowed"
              title={!topologyReady ? '请先成功加载拓扑' : !dirty ? '没有变更' : undefined}
            >
              {saving ? '保存中…' : '保存拓扑'}
            </button>
          </div>
        </>
      )}
      {err && !noNodes && <p className="text-red-600 text-xs">{err}</p>}
    </div>
  )
}

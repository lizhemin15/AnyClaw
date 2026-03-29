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
  canonPair,
  canonPairNum,
  edgeKey,
  edgeKeyNum,
  filterEdgesForInstanceIds,
  filterEdgesForSlugs,
  layoutAgents,
  ensureInstanceInList,
  instanceIdSetFromNodes,
  mergeInstanceTopologyEdgesWithPeers,
  mergeInstancesWithPeers,
  normalizeEdgesKey,
  normalizeEdgesKeyNum,
} from './collabTopologyUtils'

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
  const svgRef = useRef<SVGSVGElement>(null)
  const instanceIdRef = useRef(instanceId)
  instanceIdRef.current = instanceId

  const mergeLimits = useCallback((a?: CollabLimits, b?: CollabLimits) => {
    if (b) setLimits(b)
    else if (a) setLimits(a)
  }, [])

  const [isNarrowViewport, setIsNarrowViewport] = useState(false)
  useEffect(() => {
    const mq = window.matchMedia('(max-width: 640px)')
    const apply = () => setIsNarrowViewport(mq.matches)
    apply()
    mq.addEventListener('change', apply)
    return () => mq.removeEventListener('change', apply)
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
  const layoutRadius = isNarrowViewport ? 42 : 36
  const edgeStrokeW = isNarrowViewport ? 2.75 : 1.5
  const dragStrokeW = isNarrowViewport ? 2.25 : 1.25
  const pos = useMemo(
    () => layoutAgents(agents.length, { radius: layoutRadius }),
    [agents.length, layoutRadius]
  )
  const posInst = useMemo(
    () => layoutAgents(instanceNodes.length, { radius: layoutRadius }),
    [instanceNodes.length, layoutRadius]
  )
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
            ref={panelRef}
            className="relative mx-auto w-full min-h-0 min-w-0 max-w-[min(100%,min(80vh,28rem))] aspect-square max-h-[min(80vh,28rem)] overflow-hidden select-none touch-none rounded-xl border border-slate-200/80 bg-slate-50/50"
          >
            <svg
              ref={svgRef}
              viewBox="0 0 100 100"
              preserveAspectRatio="xMidYMid meet"
              className="absolute inset-0 block h-full w-full"
              aria-hidden
            >
              {topologyReady &&
                (isInstanceMode
                  ? displayInstEdges.map(([lo, hi]) => {
                      const i = instIdToIndex.get(lo)
                      const j = instIdToIndex.get(hi)
                      if (i == null || j == null) return null
                      const p = posInst[i]
                      const q = posInst[j]
                      return (
                        <line
                          key={`${lo}-${hi}`}
                          x1={p.x}
                          y1={p.y}
                          x2={q.x}
                          y2={q.y}
                          stroke="#818cf8"
                          strokeWidth={edgeStrokeW}
                          strokeLinecap="round"
                        />
                      )
                    })
                  : displayEdges.map(([lo, hi]) => {
                      const i = slugToIndex.get(lo)
                      const j = slugToIndex.get(hi)
                      if (i == null || j == null) return null
                      const p = pos[i]
                      const q = pos[j]
                      return (
                        <line
                          key={`${lo}-${hi}`}
                          x1={p.x}
                          y1={p.y}
                          x2={q.x}
                          y2={q.y}
                          stroke="#818cf8"
                          strokeWidth={edgeStrokeW}
                          strokeLinecap="round"
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
                    const p = posInst[i]
                    return (
                      <line
                        x1={p.x}
                        y1={p.y}
                        x2={dragCur.x}
                        y2={dragCur.y}
                        stroke="#a5b4fc"
                        strokeWidth={dragStrokeW}
                        strokeDasharray="3 2"
                        strokeLinecap="round"
                      />
                    )
                  }
                  const i = slugToIndex.get(dragFrom)
                  if (i == null) return null
                  const p = pos[i]
                  return (
                    <line
                      x1={p.x}
                      y1={p.y}
                      x2={dragCur.x}
                      y2={dragCur.y}
                      stroke="#a5b4fc"
                      strokeWidth={dragStrokeW}
                      strokeDasharray="3 2"
                      strokeLinecap="round"
                    />
                  )
                })()}
            </svg>
            {isInstanceMode
              ? instanceNodes.map((inst, i) => {
                  const { x, y } = posInst[i]
                  const key = String(inst.id)
                  const selected = pendingSlug === key
                  const nodeBusy = dragFrom === key
                  return (
                    <div
                      key={inst.id}
                      data-collab-node-slug={key}
                      className="absolute -translate-x-1/2 -translate-y-1/2 z-[1] w-[38%] max-w-[176px] min-h-[68px] sm:w-[28%] sm:max-w-[130px] sm:min-h-[56px]"
                      style={{ left: `${x}%`, top: `${y}%` }}
                    >
                      <button
                        type="button"
                        aria-pressed={selected}
                        disabled={!topologyReady}
                        onPointerDown={(e) => onNodePointerDown(key, e)}
                        className={`w-full min-h-[60px] sm:min-h-[52px] rounded-xl border-2 flex flex-col items-center justify-center px-1.5 py-2 sm:px-1 sm:py-1.5 text-center transition-shadow disabled:opacity-50 disabled:cursor-not-allowed ${
                          selected || nodeBusy
                            ? 'border-indigo-500 bg-indigo-50 shadow-md z-10'
                            : 'border-slate-200 bg-white hover:border-indigo-300 hover:bg-slate-50'
                        }`}
                        title={`${inst.name} — 拖拽到另一节点连线，或点击与另一节点配对`}
                      >
                        <span
                          className={`text-xs sm:text-[11px] font-medium line-clamp-2 leading-tight rounded-md px-1.5 py-0.5 shadow-sm max-w-full ${
                            selected || nodeBusy
                              ? 'text-indigo-950 bg-indigo-100/95'
                              : 'text-slate-800 bg-white/95'
                          }`}
                        >
                          {inst.name}
                        </span>
                        <span
                          className={`text-[11px] sm:text-[10px] truncate w-full mt-1 sm:mt-0.5 rounded px-1.5 py-0.5 shadow-sm ${
                            selected || nodeBusy ? 'text-indigo-700 bg-indigo-100/90' : 'text-slate-500 bg-slate-100/95'
                          }`}
                        >
                          #{inst.id}
                        </span>
                      </button>
                    </div>
                  )
                })
              : agents.map((a, i) => {
                  const { x, y } = pos[i]
                  const slug = a.agent_slug.trim()
                  const selected = pendingSlug === slug
                  const nodeBusy = dragFrom === slug
                  return (
                    <div
                      key={`${a.id}-${slug}`}
                      data-collab-node-slug={slug}
                      className="absolute -translate-x-1/2 -translate-y-1/2 z-[1] w-[38%] max-w-[176px] min-h-[68px] sm:w-[28%] sm:max-w-[130px] sm:min-h-[56px]"
                      style={{ left: `${x}%`, top: `${y}%` }}
                    >
                      <button
                        type="button"
                        aria-pressed={selected}
                        disabled={!topologyReady}
                        onPointerDown={(e) => onNodePointerDown(slug, e)}
                        className={`w-full min-h-[60px] sm:min-h-[52px] rounded-xl border-2 flex flex-col items-center justify-center px-1.5 py-2 sm:px-1 sm:py-1.5 text-center transition-shadow disabled:opacity-50 disabled:cursor-not-allowed ${
                          selected || nodeBusy
                            ? 'border-indigo-500 bg-indigo-50 shadow-md z-10'
                            : 'border-slate-200 bg-white hover:border-indigo-300 hover:bg-slate-50'
                        }`}
                        title={`${a.agent_slug} — 拖拽到另一节点连线，或点击与另一节点配对`}
                      >
                        <span
                          className={`text-xs sm:text-[11px] font-medium line-clamp-2 leading-tight rounded-md px-1.5 py-0.5 shadow-sm max-w-full ${
                            selected || nodeBusy
                              ? 'text-indigo-950 bg-indigo-100/95'
                              : 'text-slate-800 bg-white/95'
                          }`}
                        >
                          {a.display_name}
                        </span>
                        <span
                          className={`text-[11px] sm:text-[10px] truncate w-full mt-1 sm:mt-0.5 rounded px-1.5 py-0.5 shadow-sm ${
                            selected || nodeBusy ? 'text-indigo-700 bg-indigo-100/90' : 'text-slate-500 bg-slate-100/95'
                          }`}
                        >
                          {a.agent_slug}
                        </span>
                      </button>
                    </div>
                  )
                })}
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

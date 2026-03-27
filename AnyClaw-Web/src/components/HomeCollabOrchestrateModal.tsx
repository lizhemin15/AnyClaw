import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  ANYCLAW_COLLAB_BROADCAST,
  broadcastCollabEvent,
  getCollabAgents,
  getCollabMails,
  getCollabTopology,
  putCollabTopology,
  type CollabAgent,
  type CollabApiError,
  type CollabLimits,
  type InternalMailRow,
} from '../api'

export type HomeCollabOrchestrateModalProps = {
  open: boolean
  instanceId: number
  instanceName: string
  onClose: () => void
  onSaved?: () => void
  /** 打开时默认子标签（如从对话页直达邮件） */
  initialCollabTab?: 'topo' | 'mails'
  /** modal 为弹层；inline 为页面内嵌，仅拓扑（邮件请使用首页「邮箱」） */
  variant?: 'modal' | 'inline'
}

function canonPair(a: string, b: string): [string, string] {
  const x = a.trim()
  const y = b.trim()
  return x < y ? [x, y] : [y, x]
}

function edgeKey(lo: string, hi: string): string {
  return `${lo}\0${hi}`
}

function normalizeEdgesKey(list: [string, string][]): string {
  return [...list]
    .map(([a, b]) => edgeKey(...canonPair(a, b)))
    .sort()
    .join('|')
}

function layoutAgents(n: number): { x: number; y: number }[] {
  if (n <= 0) return []
  if (n === 1) return [{ x: 50, y: 50 }]
  return Array.from({ length: n }, (_, i) => {
    const angle = (2 * Math.PI * i) / n - Math.PI / 2
    return { x: 50 + 36 * Math.cos(angle), y: 50 + 36 * Math.sin(angle) }
  })
}

function filterEdgesForSlugs(edges: [string, string][], slugs: Set<string>): [string, string][] {
  return edges
    .map(([x, y]) => canonPair(x, y))
    .filter(([lo, hi]) => slugs.has(lo) && slugs.has(hi))
}

export default function HomeCollabOrchestrateModal({
  open,
  instanceId,
  instanceName,
  onClose,
  onSaved,
  initialCollabTab = 'topo',
  variant = 'modal',
}: HomeCollabOrchestrateModalProps) {
  const [collabTab, setCollabTab] = useState<'topo' | 'mails'>(initialCollabTab)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [loadWarn, setLoadWarn] = useState<string | null>(null)
  const [agents, setAgents] = useState<CollabAgent[]>([])
  const [limits, setLimits] = useState<CollabLimits | null>(null)
  const [edges, setEdges] = useState<[string, string][]>([])
  const [baselineEdges, setBaselineEdges] = useState<[string, string][]>([])
  const [pendingSlug, setPendingSlug] = useState<string | null>(null)
  const [topoVersion, setTopoVersion] = useState(0)
  const [topologyReady, setTopologyReady] = useState(false)
  const [staleRemote, setStaleRemote] = useState(false)
  const [mails, setMails] = useState<InternalMailRow[]>([])
  const [mailLoading, setMailLoading] = useState(false)
  const [mailLoadingMore, setMailLoadingMore] = useState(false)
  const [mailErr, setMailErr] = useState<string | null>(null)
  const [mailHasMore, setMailHasMore] = useState(false)
  const mailNextOffsetRef = useRef(0)
  const panelRef = useRef<HTMLDivElement>(null)
  /** 防止快速切换实例时，较早发起的 load 覆盖较晚实例的界面 */
  const instanceIdRef = useRef(instanceId)
  instanceIdRef.current = instanceId

  useEffect(() => {
    if (!open) return
    if (variant === 'inline') {
      setCollabTab('topo')
      return
    }
    setCollabTab(initialCollabTab === 'mails' ? 'mails' : 'topo')
  }, [open, instanceId, initialCollabTab, variant])

  const maxMailListLimit = limits?.max_internal_mail_list_limit ?? 500
  const maxMailListOffsetCap = limits?.max_internal_mail_list_offset ?? 500_000
  const mailPageSize = Math.min(100, Math.max(1, maxMailListLimit))

  const loadMails = useCallback(
    async (append: boolean) => {
      const expectedId = instanceId
      if (!append) {
        setMailLoading(true)
        setMailErr(null)
      }
      try {
        const off = append ? mailNextOffsetRef.current : 0
        if (off > maxMailListOffsetCap) {
          setMailErr(`邮件列表 offset 超过上限（${maxMailListOffsetCap}）。`)
          if (!append) {
            setMails([])
            mailNextOffsetRef.current = 0
            setMailHasMore(false)
          }
          return
        }
        const { mails: list, total, limits: ml } = await getCollabMails(expectedId, {
          limit: mailPageSize,
          offset: off,
        })
        if (instanceIdRef.current !== expectedId) return
        if (ml) setLimits(ml)
        const batch = list || []
        const totalN = typeof total === 'number' && Number.isFinite(total) ? total : null
        const newOff = off + batch.length
        if (append) {
          setMails((prev) => [...prev, ...batch])
          mailNextOffsetRef.current = newOff
        } else {
          setMails(batch)
          mailNextOffsetRef.current = newOff
        }
        if (totalN != null && totalN >= 0) {
          setMailHasMore(newOff < totalN)
        } else {
          setMailHasMore(batch.length >= mailPageSize && newOff <= maxMailListOffsetCap)
        }
        setMailErr(null)
      } catch (e) {
        if (instanceIdRef.current !== expectedId) return
        const lim = (e as CollabApiError).collabLimits
        if (lim) setLimits(lim)
        setMailErr(e instanceof Error ? e.message : String(e))
        if (!append) {
          setMails([])
          mailNextOffsetRef.current = 0
          setMailHasMore(false)
        }
      } finally {
        if (instanceIdRef.current === expectedId && !append) {
          setMailLoading(false)
        }
      }
    },
    [instanceId, mailPageSize, maxMailListOffsetCap]
  )

  const loadMoreMails = useCallback(async () => {
    if (mailLoadingMore || mailLoading || !mailHasMore) return
    setMailLoadingMore(true)
    try {
      await loadMails(true)
    } finally {
      setMailLoadingMore(false)
    }
  }, [loadMails, mailHasMore, mailLoading, mailLoadingMore])

  useEffect(() => {
    if (!open || collabTab !== 'mails') return
    void loadMails(false)
  }, [open, collabTab, instanceId, loadMails])

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
    setLimits(null)

    const settled = await Promise.allSettled([
      getCollabAgents(expectedId),
      getCollabTopology(expectedId),
    ])

    if (instanceIdRef.current !== expectedId) {
      setLoading(false)
      return
    }

    const ar = settled[0]
    const tr = settled[1]

    if (ar.status === 'rejected') {
      if (instanceIdRef.current !== expectedId) {
        setLoading(false)
        return
      }
      const r = ar.reason
      const msg = r instanceof Error ? r.message : String(r)
      if (r instanceof Error) {
        const lim = (r as CollabApiError).collabLimits
        if (lim) setLimits(lim)
      }
      setErr(msg)
      setAgents([])
      setEdges([])
      setBaselineEdges([])
      setTopoVersion(0)
      setLoading(false)
      return
    }

    if (instanceIdRef.current !== expectedId) {
      setLoading(false)
      return
    }

    const a = ar.value
    const list = a.agents || []
    setAgents(list)
    mergeLimits(undefined, a.limits)

    if (tr.status === 'rejected') {
      if (instanceIdRef.current !== expectedId) {
        setLoading(false)
        return
      }
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

    if (instanceIdRef.current !== expectedId) {
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
  }, [instanceId, mergeLimits])

  useEffect(() => {
    if (!open) return
    void load()
  }, [open, load])

  const slugSet = useMemo(() => new Set(agents.map((a) => a.agent_slug.trim())), [agents])

  const dirty = useMemo(() => {
    if (agents.length < 2 || !topologyReady) return false
    return normalizeEdgesKey(edges) !== normalizeEdgesKey(baselineEdges)
  }, [agents.length, edges, baselineEdges, topologyReady])

  const dirtyRef = useRef(false)
  dirtyRef.current = dirty
  const collabTabRef = useRef(collabTab)
  collabTabRef.current = collabTab

  const requestClose = useCallback(() => {
    if (dirty) {
      if (!confirm('有未保存的拓扑变更，确定关闭？')) return
    }
    onClose()
  }, [dirty, onClose])

  useEffect(() => {
    if (!open || variant === 'inline') return
    const prevOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = prevOverflow
    }
  }, [open, variant])

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        requestClose()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [open, requestClose])

  useEffect(() => {
    if (!open || loading || variant === 'inline') return
    const t = window.setTimeout(() => {
      panelRef.current?.focus()
    }, 50)
    return () => clearTimeout(t)
  }, [open, loading, variant])

  useEffect(() => {
    if (!open || typeof BroadcastChannel === 'undefined') return
    let bc: BroadcastChannel | null = null
    try {
      bc = new BroadcastChannel(ANYCLAW_COLLAB_BROADCAST)
      bc.onmessage = (ev: MessageEvent) => {
        const d = ev.data as { kind?: string; instanceId?: number }
        if (d.instanceId !== instanceId) return
        if (d.kind === 'internal_mail' && collabTabRef.current === 'mails') {
          void loadMails(false)
          return
        }
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
  }, [open, instanceId, load, loadMails])

  const maxEdges = limits?.max_edges ?? 4096
  const pos = useMemo(() => layoutAgents(agents.length), [agents.length])
  const slugToIndex = useMemo(() => {
    const m = new Map<string, number>()
    agents.forEach((a, i) => m.set(a.agent_slug.trim(), i))
    return m
  }, [agents])

  const edgeSet = useMemo(() => {
    const s = new Set<string>()
    for (const [lo, hi] of edges) s.add(edgeKey(lo, hi))
    return s
  }, [edges])

  const displayEdges = useMemo(
    () => edges.filter(([lo, hi]) => slugSet.has(lo) && slugSet.has(hi)),
    [edges, slugSet]
  )

  const toggleEdge = (slugA: string, slugB: string) => {
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
  }

  const onNodeClick = (slug: string) => {
    if (!topologyReady) return
    if (!pendingSlug) {
      setPendingSlug(slug)
      return
    }
    if (pendingSlug === slug) {
      setPendingSlug(null)
      return
    }
    toggleEdge(pendingSlug, slug)
    setPendingSlug(null)
  }

  const handleRestore = () => {
    setEdges([...baselineEdges])
    setPendingSlug(null)
    setErr(null)
    setStaleRemote(false)
  }

  const handleSave = async () => {
    if (!topologyReady || agents.length < 2) return
    const cleaned = filterEdgesForSlugs(edges, slugSet)
    setSaving(true)
    setErr(null)
    try {
      await putCollabTopology(instanceId, cleaned)
      setBaselineEdges(cleaned)
      setEdges(cleaned)
      broadcastCollabEvent('topology', instanceId)
      onSaved?.()
      if (variant === 'modal') onClose()
    } catch (e) {
      const lim = (e as CollabApiError).collabLimits
      if (lim) setLimits(lim)
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  const orchDescId = 'home-orch-desc'
  const canSave = topologyReady && agents.length > 1 && dirty && !saving && !loading

  if (!open) return null

  const panelClassName =
    variant === 'inline'
      ? 'bg-white rounded-2xl shadow-sm w-full max-w-4xl flex flex-col border border-violet-200 outline-none'
      : 'bg-white rounded-2xl shadow-xl max-w-2xl w-full max-h-[90vh] flex flex-col border border-slate-200 outline-none ring-offset-2 focus-visible:ring-2 focus-visible:ring-violet-400'

  return (
    <div
      className={variant === 'modal' ? 'fixed inset-0 z-[60] flex items-center justify-center p-4 bg-black/50' : 'w-full'}
      role={variant === 'modal' ? 'dialog' : undefined}
      aria-modal={variant === 'modal' ? true : undefined}
      aria-describedby={variant === 'modal' ? orchDescId : undefined}
      onClick={variant === 'modal' ? () => requestClose() : undefined}
    >
      <div
        ref={panelRef}
        tabIndex={variant === 'modal' ? -1 : undefined}
        className={panelClassName}
        onClick={variant === 'modal' ? (e) => e.stopPropagation() : undefined}
        role="document"
        aria-labelledby="home-orch-title"
      >
        <div className="px-5 pt-4 pb-2 border-b border-slate-100">
          <div className="flex flex-wrap items-start justify-between gap-2">
            <h2 id="home-orch-title" className="text-lg font-semibold text-slate-800">
              协作 · {instanceName || `#${instanceId}`}
            </h2>
            {collabTab === 'topo' && agents.length > 1 && !loading && topologyReady && (
              <span className="text-xs tabular-nums text-slate-500 bg-slate-100 px-2 py-0.5 rounded-md">
                连线 {edges.length}/{maxEdges}
                {topoVersion > 0 ? ` · v${topoVersion}` : ''}
                {dirty ? ' · 未保存' : ''}
              </span>
            )}
          </div>
          {variant !== 'inline' && (
            <div className="flex gap-1 p-1 bg-slate-100 rounded-xl w-fit mt-3">
              <button
                type="button"
                onClick={() => setCollabTab('topo')}
                className={`px-4 py-2 rounded-lg text-sm font-medium transition-colors ${
                  collabTab === 'topo' ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-600 hover:text-slate-900'
                }`}
              >
                编排
              </button>
              <button
                type="button"
                onClick={() => setCollabTab('mails')}
                className={`px-4 py-2 rounded-lg text-sm font-medium transition-colors ${
                  collabTab === 'mails' ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-600 hover:text-slate-900'
                }`}
              >
                邮件
              </button>
            </div>
          )}
          <p id={orchDescId} className="text-xs text-slate-500 mt-2">
            {variant === 'inline' ? (
              <>
                依次点击两个节点添加或移除连线（无向邻居）。员工列表由容器 <code className="bg-slate-100 px-0.5 rounded">agents.list</code> 启动时自动同步。内部邮件汇总请点上方「邮箱」。
              </>
            ) : collabTab === 'topo' ? (
              <>
                依次点击两个节点添加或移除连线（无向邻居）。员工列表由容器 <code className="bg-slate-100 px-0.5 rounded">agents.list</code> 启动时自动同步。
              </>
            ) : (
              <>员工之间往来的内部邮件，按时间倒序，最新在最上。按 Esc 关闭。</>
            )}
          </p>
        </div>

        <div className="px-5 py-4 flex-1 overflow-y-auto min-h-0">
          {collabTab === 'mails' && variant !== 'inline' ? (
            <div className="space-y-3">
              {mailErr && (
                <div className="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-800">{mailErr}</div>
              )}
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="text-xs text-slate-500">本实例全部内部邮件，最新在最上。</p>
                <button
                  type="button"
                  disabled={mailLoading}
                  onClick={() => void loadMails(false)}
                  className="text-xs px-3 py-1.5 rounded-lg border border-slate-200 hover:bg-slate-50 disabled:opacity-50"
                >
                  {mailLoading ? '刷新中…' : '刷新'}
                </button>
              </div>
              {mailLoading && mails.length === 0 ? (
                <p className="text-slate-500 text-sm py-12 text-center">加载邮件…</p>
              ) : mails.length === 0 ? (
                <p className="text-slate-400 text-sm py-8 text-center">暂无内部邮件</p>
              ) : (
                <div className="space-y-3">
                  {mails.map((m) => (
                    <div key={m.id} className="border border-slate-200 rounded-xl p-3 bg-slate-50/40 space-y-1.5">
                      <div className="flex flex-wrap gap-x-2 gap-y-0.5 text-xs text-slate-500">
                        <span className="font-mono">#{m.id}</span>
                        <span>{m.created_at}</span>
                        <span>
                          {m.from_slug} → {m.to_slug}
                        </span>
                        {m.thread_id ? (
                          <code
                            className="text-[10px] bg-slate-200/80 px-1 rounded max-w-[200px] truncate"
                            title={m.thread_id}
                          >
                            {m.thread_id}
                          </code>
                        ) : null}
                        {m.in_reply_to != null && <span>↩ {m.in_reply_to}</span>}
                      </div>
                      <div className="font-medium text-slate-800 text-sm">{m.subject || '—'}</div>
                      <div className="text-xs text-slate-600 whitespace-pre-wrap break-words">{m.body}</div>
                    </div>
                  ))}
                </div>
              )}
              {mailHasMore && mails.length > 0 && (
                <button
                  type="button"
                  disabled={mailLoadingMore}
                  onClick={() => void loadMoreMails()}
                  className="w-full py-2 rounded-lg border border-slate-200 text-sm text-slate-700 hover:bg-slate-50 disabled:opacity-50"
                >
                  {mailLoadingMore ? '加载中…' : '加载更早的邮件'}
                </button>
              )}
            </div>
          ) : (
            <>
          {staleRemote && (
            <div className="mb-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900 flex flex-wrap items-center justify-between gap-2">
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
            <div className="mb-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900">{loadWarn}</div>
          )}

          {loading ? (
            <p className="text-slate-500 text-sm py-12 text-center">加载中…</p>
          ) : err && agents.length === 0 ? (
            <div className="py-8 text-center space-y-3">
              <p className="text-red-600 text-sm">{err}</p>
              <button
                type="button"
                onClick={() => void load()}
                className="text-sm text-indigo-600 px-3 py-1.5 border border-indigo-200 rounded-lg hover:bg-indigo-50"
              >
                重新加载
              </button>
            </div>
          ) : agents.length === 0 ? (
            <p className="text-slate-500 text-sm py-8 text-center">暂无协作成员数据</p>
          ) : agents.length === 1 ? (
            <div className="space-y-3">
              <p className="text-sm text-amber-800 bg-amber-50 border border-amber-100 rounded-lg px-3 py-2">
                当前仅一名协作成员。若容器已配置多名 <code className="bg-amber-100 px-0.5 rounded">agents.list</code>，请待员工进程连上云端后会自动补全；也可在{' '}
                <Link to={`/instances/${instanceId}/collab`} className="text-amber-900 underline" onClick={() => onClose()}>
                  展示名设置
                </Link>{' '}
                中核对。
              </p>
              <div className="relative w-full aspect-square max-h-64 mx-auto">
                <svg viewBox="0 0 100 100" className="w-full h-full" aria-hidden>
                  <circle cx={50} cy={50} r={14} fill="#e0e7ff" stroke="#6366f1" strokeWidth={1.2} />
                </svg>
                <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
                  <span className="text-xs font-medium text-indigo-900 text-center px-2 max-w-[90%] truncate">
                    {agents[0].display_name}
                  </span>
                </div>
              </div>
            </div>
          ) : (
            <>
              <div className="flex flex-wrap items-center gap-2 mb-2">
                {pendingSlug && topologyReady && (
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
              {dirty && topologyReady && (
                <button
                  type="button"
                  onClick={handleRestore}
                  className="text-xs text-slate-600 mb-2 px-2 py-1 border border-dashed border-slate-300 rounded-lg hover:bg-slate-50"
                >
                  还原为上次加载
                </button>
              )}
              <div className="relative w-full aspect-square max-h-80 mx-auto select-none touch-manipulation">
                <svg viewBox="0 0 100 100" className="absolute inset-0 w-full h-full pointer-events-none" aria-hidden>
                  {topologyReady &&
                    displayEdges.map(([lo, hi]) => {
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
                          strokeWidth={1.5}
                          strokeLinecap="round"
                        />
                      )
                    })}
                </svg>
                {agents.map((a, i) => {
                  const { x, y } = pos[i]
                  const slug = a.agent_slug.trim()
                  const selected = pendingSlug === slug
                  return (
                    <button
                      key={`${a.id}-${slug}`}
                      type="button"
                      aria-pressed={selected}
                      disabled={!topologyReady}
                      onClick={() => onNodeClick(slug)}
                      className={`absolute w-[26%] max-w-[120px] min-h-[52px] -translate-x-1/2 -translate-y-1/2 rounded-xl border-2 flex flex-col items-center justify-center px-1 py-1.5 text-center transition-shadow disabled:opacity-50 disabled:cursor-not-allowed ${
                        selected
                          ? 'border-indigo-500 bg-indigo-50 shadow-md z-10'
                          : 'border-slate-200 bg-white hover:border-indigo-300 hover:bg-slate-50'
                      }`}
                      style={{ left: `${x}%`, top: `${y}%` }}
                      title={a.agent_slug}
                    >
                      <span className="text-[11px] font-medium text-slate-800 line-clamp-2 leading-tight">{a.display_name}</span>
                      <span className="text-[10px] text-slate-400 truncate w-full mt-0.5">{a.agent_slug}</span>
                    </button>
                  )
                })}
              </div>
            </>
          )}
          {err && agents.length > 0 && <p className="text-red-600 text-xs mt-3">{err}</p>}
            </>
          )}
        </div>

        <div className="px-5 py-3 border-t border-slate-100 flex flex-wrap gap-2 justify-end items-center bg-slate-50/80 rounded-b-2xl">
          <Link
            to={`/instances/${instanceId}/collab`}
            onClick={(e) => {
              if (dirty) {
                if (!confirm('有未保存的拓扑变更，离开将关闭此窗口，未保存的连线将丢失，确定？')) {
                  e.preventDefault()
                  return
                }
              }
              onClose()
            }}
            className="text-xs text-slate-500 hover:text-indigo-600 mr-auto"
          >
            修改展示名…
          </Link>
          <button
            type="button"
            onClick={() => requestClose()}
            className="px-3 py-2 text-sm text-slate-600 hover:bg-slate-200/80 rounded-lg"
          >
            关闭
          </button>
          {collabTab === 'topo' && agents.length > 1 && (
            <button
              type="button"
              disabled={!canSave}
              onClick={() => void handleSave()}
              className="px-4 py-2 text-sm bg-slate-800 text-white rounded-lg disabled:opacity-50 disabled:cursor-not-allowed"
              title={!topologyReady ? '请先成功加载拓扑' : !dirty ? '没有变更' : undefined}
            >
              {saving ? '保存中…' : '保存拓扑'}
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

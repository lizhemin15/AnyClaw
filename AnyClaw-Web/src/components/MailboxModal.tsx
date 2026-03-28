import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  getCollabMails,
  getCollabInstanceMails,
  postCollabInstanceMail,
  broadcastCollabEvent,
  type InternalMailRow,
  type Instance,
  type UserInstanceMessageRow,
} from '../api'

export type MailboxTab = 'internal' | 'cross'

type Props = {
  open: boolean
  instances: Instance[]
  onClose: () => void
  /** 打开时默认选中的标签（如从编排跳转「邮件」时用 internal） */
  initialTab?: MailboxTab
}

const INTERNAL_PAGE = 80
const CROSS_PAGE = 120

function mergeDedupe(rows: UserInstanceMessageRow[][]): UserInstanceMessageRow[] {
  const byId = new Map<number, UserInstanceMessageRow>()
  for (const batch of rows) {
    for (const m of batch) {
      if (!byId.has(m.id)) byId.set(m.id, m)
    }
  }
  return [...byId.values()].sort((a, b) => (b.created_at || '').localeCompare(a.created_at || ''))
}

export default function MailboxModal({ open, instances, onClose, initialTab = 'internal' }: Props) {
  const [tab, setTab] = useState<MailboxTab>(initialTab)

  /* —— 内部邮件 —— */
  const [internalRows, setInternalRows] = useState<InternalMailRow[]>([])
  const [internalLoading, setInternalLoading] = useState(false)
  const [internalLoadingMore, setInternalLoadingMore] = useState(false)
  const [internalErr, setInternalErr] = useState<string | null>(null)
  const [internalHasMore, setInternalHasMore] = useState(false)
  const internalOffsetRef = useRef<Record<number, number>>({})
  const internalExhaustedRef = useRef<Set<number>>(new Set())

  /* —— 跨实例 —— */
  const [crossRows, setCrossRows] = useState<UserInstanceMessageRow[]>([])
  const [crossLoading, setCrossLoading] = useState(false)
  const [crossErr, setCrossErr] = useState<string | null>(null)
  const [peerFilter, setPeerFilter] = useState<number | null>(null)
  const [sendFrom, setSendFrom] = useState<number | null>(null)
  const [sendTo, setSendTo] = useState<number | null>(null)
  const [sendBody, setSendBody] = useState('')
  const [sending, setSending] = useState(false)

  const idToName = useMemo(() => {
    const m = new Map<number, string>()
    for (const i of instances) m.set(i.id, i.name)
    return m
  }, [instances])

  const instanceIds = useMemo(() => instances.map((i) => i.id), [instances])

  const loadInternalPage = useCallback(
    async (reset: boolean) => {
      if (instanceIds.length === 0) {
        setInternalRows([])
        setInternalHasMore(false)
        if (reset) setInternalLoading(false)
        return
      }
      if (reset) {
        internalOffsetRef.current = {}
        internalExhaustedRef.current = new Set()
        setInternalLoading(true)
        setInternalErr(null)
      }
      try {
        const offs = internalOffsetRef.current
        const ids = instanceIds.filter((id) => !internalExhaustedRef.current.has(id))
        if (ids.length === 0) {
          setInternalHasMore(false)
          if (reset) setInternalRows([])
          return
        }
        const settled = await Promise.allSettled(
          ids.map((id) => getCollabMails(id, { limit: INTERNAL_PAGE, offset: offs[id] ?? 0 }))
        )
        const batch: InternalMailRow[] = []
        for (let i = 0; i < settled.length; i++) {
          const id = ids[i]
          const r = settled[i]
          if (r.status !== 'fulfilled') continue
          const list = r.value.mails || []
          const total = r.value.total
          offs[id] = (offs[id] ?? 0) + list.length
          batch.push(...list)
          if (list.length < INTERNAL_PAGE || (typeof total === 'number' && offs[id] >= total)) {
            internalExhaustedRef.current.add(id)
          }
        }
        if (reset) {
          setInternalRows(batch)
        } else {
          setInternalRows((prev) => [...prev, ...batch])
        }
        const anyLeft = instanceIds.some((id) => !internalExhaustedRef.current.has(id))
        setInternalHasMore(anyLeft)
        setInternalErr(null)
      } catch (e) {
        setInternalErr(e instanceof Error ? e.message : String(e))
        if (reset) setInternalRows([])
      } finally {
        if (reset) setInternalLoading(false)
      }
    },
    [instanceIds]
  )

  const loadCrossAll = useCallback(async () => {
    if (instanceIds.length === 0) {
      setCrossRows([])
      return
    }
    setCrossLoading(true)
    setCrossErr(null)
    try {
      const settled = await Promise.allSettled(
        instanceIds.map((id) => getCollabInstanceMails(id, { limit: CROSS_PAGE, offset: 0 }))
      )
      const batches: UserInstanceMessageRow[][] = []
      for (let i = 0; i < settled.length; i++) {
        const r = settled[i]
        if (r.status === 'fulfilled') batches.push(r.value.messages || [])
      }
      setCrossRows(mergeDedupe(batches))
    } catch (e) {
      setCrossErr(e instanceof Error ? e.message : String(e))
      setCrossRows([])
    } finally {
      setCrossLoading(false)
    }
  }, [instanceIds])

  useEffect(() => {
    if (!open) return
    setTab(initialTab)
  }, [open, initialTab])

  useEffect(() => {
    if (!open || tab !== 'internal') return
    void loadInternalPage(true)
  }, [open, tab, loadInternalPage])

  useEffect(() => {
    if (!open || tab !== 'cross') return
    void loadCrossAll()
  }, [open, tab, loadCrossAll])

  useEffect(() => {
    if (!open) {
      setPeerFilter(null)
      setSendBody('')
    }
  }, [open])

  useEffect(() => {
    if (!open) return
    if (instances.length >= 2) {
      setSendFrom(instances[0].id)
      setSendTo(instances[1].id)
    } else if (instances.length === 1) {
      setSendFrom(instances[0].id)
      setSendTo(null)
    }
  }, [open, instances])

  useEffect(() => {
    if (!open) return
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = prev
    }
  }, [open])

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        onClose()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [open, onClose])

  const internalMergedSorted = useMemo(() => {
    const list = [...internalRows]
    list.sort((a, b) => (b.created_at || '').localeCompare(a.created_at || ''))
    return list
  }, [internalRows])

  const loadMoreInternal = async () => {
    if (internalLoadingMore || internalLoading || !internalHasMore) return
    setInternalLoadingMore(true)
    try {
      await loadInternalPage(false)
    } finally {
      setInternalLoadingMore(false)
    }
  }

  const peerActivity = useMemo(() => {
    const m = new Map<number, string>()
    for (const row of crossRows) {
      for (const pid of [row.from_instance_id, row.to_instance_id]) {
        const t = row.created_at || ''
        const prev = m.get(pid)
        if (!prev || t > prev) m.set(pid, t)
      }
    }
    return m
  }, [crossRows])

  const peerIdsSorted = useMemo(() => {
    const ids = [...peerActivity.keys()]
    ids.sort((a, b) => (peerActivity.get(b) || '').localeCompare(peerActivity.get(a) || ''))
    return ids
  }, [peerActivity])

  const crossFiltered = useMemo(() => {
    if (peerFilter == null) return crossRows
    return crossRows.filter((m) => m.from_instance_id === peerFilter || m.to_instance_id === peerFilter)
  }, [crossRows, peerFilter])

  const handleSendCross = async (e: React.FormEvent) => {
    e.preventDefault()
    if (sending || sendFrom == null || sendTo == null || sendFrom === sendTo) return
    const content = sendBody.trim()
    if (!content) return
    setSending(true)
    setCrossErr(null)
    try {
      await postCollabInstanceMail(sendFrom, { to_instance_id: sendTo, content })
      broadcastCollabEvent('instance_mail', sendFrom)
      broadcastCollabEvent('instance_mail', sendTo)
      setSendBody('')
      await loadCrossAll()
    } catch (e2) {
      setCrossErr(e2 instanceof Error ? e2.message : String(e2))
    } finally {
      setSending(false)
    }
  }

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center p-4 bg-black/50"
      role="dialog"
      aria-modal="true"
      aria-labelledby="mailbox-title"
      onClick={() => onClose()}
    >
      <div
        className="bg-white rounded-2xl shadow-xl max-w-3xl w-full max-h-[90vh] flex flex-col border border-slate-200"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="px-5 pt-4 pb-2 border-b border-slate-100">
          <h2 id="mailbox-title" className="text-lg font-semibold text-slate-800">
            信箱
          </h2>
          <p className="text-xs text-slate-500 mt-2">内部邮件与跨实例消息统一管理。按 Esc 关闭。</p>
          <div className="flex gap-1 mt-3" role="tablist" aria-label="信箱类型">
            <button
              type="button"
              role="tab"
              aria-selected={tab === 'internal'}
              onClick={() => setTab('internal')}
              className={`px-3 py-1.5 text-sm rounded-lg border transition-colors ${
                tab === 'internal'
                  ? 'border-violet-500 bg-violet-50 text-violet-900 font-medium'
                  : 'border-slate-200 text-slate-600 hover:bg-slate-50'
              }`}
            >
              内部邮件
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={tab === 'cross'}
              onClick={() => setTab('cross')}
              className={`px-3 py-1.5 text-sm rounded-lg border transition-colors ${
                tab === 'cross'
                  ? 'border-indigo-500 bg-indigo-50 text-indigo-900 font-medium'
                  : 'border-slate-200 text-slate-600 hover:bg-slate-50'
              }`}
            >
              跨实例消息
            </button>
          </div>
        </div>

        {tab === 'internal' && (
          <div className="px-5 py-4 flex-1 overflow-y-auto min-h-0">
            {internalErr && (
              <div className="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-800 mb-3">{internalErr}</div>
            )}
            <div className="flex flex-wrap items-center justify-between gap-2 mb-3">
              <p className="text-xs text-slate-500">同实例内员工往来 · 共 {instances.length} 名员工（实例）</p>
              <button
                type="button"
                disabled={internalLoading}
                onClick={() => void loadInternalPage(true)}
                className="text-xs px-3 py-1.5 rounded-lg border border-slate-200 hover:bg-slate-50 disabled:opacity-50"
              >
                {internalLoading ? '刷新中…' : '刷新'}
              </button>
            </div>
            {internalLoading && internalMergedSorted.length === 0 ? (
              <p className="text-slate-500 text-sm py-12 text-center">加载邮件…</p>
            ) : internalMergedSorted.length === 0 ? (
              <p className="text-slate-400 text-sm py-8 text-center">暂无内部邮件</p>
            ) : (
              <div className="space-y-3">
                {internalMergedSorted.map((m) => (
                  <div key={`${m.instance_id}-${m.id}`} className="border border-slate-200 rounded-xl p-3 bg-slate-50/40 space-y-1.5">
                    <div className="flex flex-wrap gap-x-2 gap-y-0.5 text-xs text-slate-500">
                      <span className="font-medium text-violet-700">{idToName.get(m.instance_id) ?? `#${m.instance_id}`}</span>
                      <span className="font-mono">#{m.id}</span>
                      <span>{m.created_at}</span>
                      <span>
                        {m.from_slug} → {m.to_slug}
                      </span>
                      {m.thread_id ? (
                        <code className="text-[10px] bg-slate-200/80 px-1 rounded max-w-[200px] truncate" title={m.thread_id}>
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
            {internalHasMore && internalMergedSorted.length > 0 && (
              <button
                type="button"
                disabled={internalLoadingMore}
                onClick={() => void loadMoreInternal()}
                className="w-full mt-3 py-2 rounded-lg border border-slate-200 text-sm text-slate-700 hover:bg-slate-50 disabled:opacity-50"
              >
                {internalLoadingMore ? '加载中…' : '加载更多'}
              </button>
            )}
          </div>
        )}

        {tab === 'cross' && (
          <>
            <div className="flex flex-1 min-h-0 flex-col sm:flex-row">
              <div className="sm:w-44 flex-shrink-0 border-b sm:border-b-0 sm:border-r border-slate-100 p-3 overflow-y-auto max-h-32 sm:max-h-none">
                <button
                  type="button"
                  onClick={() => setPeerFilter(null)}
                  className={`w-full text-left px-2 py-1.5 rounded-lg text-sm mb-1 ${
                    peerFilter == null ? 'bg-indigo-50 text-indigo-900 font-medium' : 'text-slate-600 hover:bg-slate-50'
                  }`}
                >
                  全部
                </button>
                {peerIdsSorted.map((pid) => (
                  <button
                    key={pid}
                    type="button"
                    onClick={() => setPeerFilter(pid)}
                    className={`w-full text-left px-2 py-1.5 rounded-lg text-sm mb-1 truncate ${
                      peerFilter === pid ? 'bg-indigo-50 text-indigo-900 font-medium' : 'text-slate-600 hover:bg-slate-50'
                    }`}
                    title={idToName.get(pid) ?? `#${pid}`}
                  >
                    {idToName.get(pid) ?? `#${pid}`}
                  </button>
                ))}
              </div>
              <div className="flex-1 px-5 py-4 overflow-y-auto min-h-0">
                {crossErr && (
                  <div className="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-800 mb-3">{crossErr}</div>
                )}
                <div className="flex flex-wrap items-center justify-between gap-2 mb-3">
                  <p className="text-xs text-slate-500">
                    账号下实例之间 · {peerFilter == null ? `共 ${crossFiltered.length} 条` : `与「${idToName.get(peerFilter) ?? peerFilter}」相关 ${crossFiltered.length} 条`}
                  </p>
                  <button
                    type="button"
                    disabled={crossLoading}
                    onClick={() => void loadCrossAll()}
                    className="text-xs px-3 py-1.5 rounded-lg border border-slate-200 hover:bg-slate-50 disabled:opacity-50"
                  >
                    {crossLoading ? '刷新中…' : '刷新'}
                  </button>
                </div>
                {crossLoading && crossFiltered.length === 0 ? (
                  <p className="text-slate-500 text-sm py-12 text-center">加载中…</p>
                ) : crossFiltered.length === 0 ? (
                  <p className="text-slate-400 text-sm py-8 text-center">暂无跨实例消息</p>
                ) : (
                  <div className="space-y-3">
                    {crossFiltered.map((m) => {
                      const fromName = idToName.get(m.from_instance_id) ?? `#${m.from_instance_id}`
                      const toName = idToName.get(m.to_instance_id) ?? `#${m.to_instance_id}`
                      return (
                        <div key={m.id} className="border border-slate-200 rounded-xl p-3 bg-slate-50/40 space-y-1.5">
                          <div className="flex flex-wrap gap-x-2 gap-y-0.5 text-xs text-slate-500">
                            <span className="font-mono">#{m.id}</span>
                            <span>{m.created_at}</span>
                          </div>
                          <div className="text-sm text-slate-800">
                            <span className="font-medium text-indigo-800">{fromName}</span>
                            <span className="text-slate-400 mx-1">→</span>
                            <span className="font-medium text-indigo-800">{toName}</span>
                          </div>
                          <div className="text-xs text-slate-600 whitespace-pre-wrap break-words">{m.content}</div>
                        </div>
                      )
                    })}
                  </div>
                )}
              </div>
            </div>
            {instances.length >= 2 && (
              <form onSubmit={handleSendCross} className="px-5 py-3 border-t border-slate-100 bg-slate-50/80 space-y-2">
                <p className="text-xs text-slate-500">发送跨实例消息（需在编排拓扑中已连线）</p>
                <div className="flex flex-col sm:flex-row gap-2 sm:items-center">
                  <label className="text-xs text-slate-600 flex items-center gap-1">
                    从
                    <select
                      value={sendFrom ?? ''}
                      onChange={(e) => setSendFrom(Number(e.target.value) || null)}
                      className="border border-slate-200 rounded-lg px-2 py-1 text-sm max-w-[140px]"
                    >
                      {instances.map((i) => (
                        <option key={i.id} value={i.id}>
                          {i.name}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label className="text-xs text-slate-600 flex items-center gap-1">
                    发往
                    <select
                      value={sendTo ?? ''}
                      onChange={(e) => setSendTo(Number(e.target.value) || null)}
                      className="border border-slate-200 rounded-lg px-2 py-1 text-sm max-w-[140px]"
                    >
                      {instances.map((i) => (
                        <option key={i.id} value={i.id} disabled={i.id === sendFrom}>
                          {i.name}
                        </option>
                      ))}
                    </select>
                  </label>
                </div>
                <textarea
                  value={sendBody}
                  onChange={(e) => setSendBody(e.target.value)}
                  placeholder="正文"
                  rows={2}
                  className="w-full border border-slate-200 rounded-lg px-3 py-2 text-sm"
                />
                <div className="flex justify-end">
                  <button
                    type="submit"
                    disabled={sending || !sendBody.trim() || sendFrom == null || sendTo == null || sendFrom === sendTo}
                    className="px-4 py-2 text-sm bg-indigo-600 text-white rounded-lg disabled:opacity-50"
                  >
                    {sending ? '发送中…' : '发送'}
                  </button>
                </div>
              </form>
            )}
          </>
        )}

        <div className="px-5 py-3 border-t border-slate-100 flex justify-end bg-white rounded-b-2xl">
          <button type="button" onClick={() => onClose()} className="px-4 py-2 text-sm bg-slate-800 text-white rounded-lg">
            关闭
          </button>
        </div>
      </div>
    </div>
  )
}

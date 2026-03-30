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

type Props = {
  open: boolean
  instances: Instance[]
  onClose: () => void
}

const PAGE_SIZE = 20
const READ_STORAGE = 'anyclaw-mailbox-read-v1'

type UnifiedItem =
  | { kind: 'internal'; row: InternalMailRow; sortKey: string }
  | { kind: 'cross'; row: UserInstanceMessageRow; sortKey: string }

function mergeDedupe(rows: UserInstanceMessageRow[][]): UserInstanceMessageRow[] {
  const byId = new Map<number, UserInstanceMessageRow>()
  for (const batch of rows) {
    for (const m of batch) {
      if (!byId.has(m.id)) byId.set(m.id, m)
    }
  }
  return [...byId.values()].sort((a, b) => (b.created_at || '').localeCompare(a.created_at || ''))
}

function itemKey(u: UnifiedItem): string {
  return u.kind === 'internal' ? `i:${u.row.instance_id}:${u.row.id}` : `c:${u.row.id}`
}

function previewText(s: string, max = 72): string {
  const line = s.replace(/\s+/g, ' ').trim()
  if (!line) return ''
  return line.length > max ? `${line.slice(0, max)}…` : line
}

function formatMailTime(iso: string): string {
  try {
    const d = new Date(iso)
    if (Number.isNaN(d.getTime())) return iso
    return d.toLocaleString('zh-CN', {
      month: 'numeric',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return iso
  }
}

export default function MailboxModal({ open, instances, onClose }: Props) {
  const [readIds, setReadIds] = useState<Set<string>>(() => new Set())
  const [selectedKey, setSelectedKey] = useState<string | null>(null)

  const [internalRows, setInternalRows] = useState<InternalMailRow[]>([])
  const [internalLoading, setInternalLoading] = useState(false)
  const [internalLoadingMore, setInternalLoadingMore] = useState(false)
  const [internalErr, setInternalErr] = useState<string | null>(null)
  const [internalHasMore, setInternalHasMore] = useState(false)
  const internalOffsetRef = useRef<Record<number, number>>({})
  const internalExhaustedRef = useRef<Set<number>>(new Set())

  const [crossRows, setCrossRows] = useState<UserInstanceMessageRow[]>([])
  const [crossLoading, setCrossLoading] = useState(false)
  const [crossLoadingMore, setCrossLoadingMore] = useState(false)
  const [crossErr, setCrossErr] = useState<string | null>(null)
  const [crossHasMore, setCrossHasMore] = useState(false)
  const crossOffsetRef = useRef<Record<number, number>>({})
  const crossExhaustedRef = useRef<Set<number>>(new Set())
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
          ids.map((id) => getCollabMails(id, { limit: PAGE_SIZE, offset: offs[id] ?? 0 }))
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
          if (list.length < PAGE_SIZE || (typeof total === 'number' && offs[id] >= total)) {
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

  const loadCrossPage = useCallback(
    async (reset: boolean) => {
      if (instanceIds.length === 0) {
        setCrossRows([])
        setCrossHasMore(false)
        if (reset) setCrossLoading(false)
        return
      }
      if (reset) {
        crossOffsetRef.current = {}
        crossExhaustedRef.current = new Set()
        setCrossLoading(true)
        setCrossErr(null)
      }
      try {
        const offs = crossOffsetRef.current
        const ids = instanceIds.filter((id) => !crossExhaustedRef.current.has(id))
        if (ids.length === 0) {
          setCrossHasMore(false)
          if (reset) setCrossRows([])
          return
        }
        const settled = await Promise.allSettled(
          ids.map((id) => getCollabInstanceMails(id, { limit: PAGE_SIZE, offset: offs[id] ?? 0 }))
        )
        const batch: UserInstanceMessageRow[] = []
        for (let i = 0; i < settled.length; i++) {
          const id = ids[i]
          const r = settled[i]
          if (r.status !== 'fulfilled') continue
          const list = r.value.messages || []
          const total = r.value.total
          offs[id] = (offs[id] ?? 0) + list.length
          batch.push(...list)
          if (list.length < PAGE_SIZE || (typeof total === 'number' && offs[id] >= total)) {
            crossExhaustedRef.current.add(id)
          }
        }
        if (reset) {
          setCrossRows(mergeDedupe([batch]))
        } else {
          setCrossRows((prev) => mergeDedupe([prev, batch]))
        }
        const anyLeft = instanceIds.some((id) => !crossExhaustedRef.current.has(id))
        setCrossHasMore(anyLeft)
        setCrossErr(null)
      } catch (e) {
        setCrossErr(e instanceof Error ? e.message : String(e))
        if (reset) setCrossRows([])
      } finally {
        if (reset) setCrossLoading(false)
      }
    },
    [instanceIds]
  )

  useEffect(() => {
    if (!open) return
    void loadInternalPage(true)
  }, [open, loadInternalPage])

  useEffect(() => {
    if (!open) return
    void loadCrossPage(true)
  }, [open, loadCrossPage])

  useEffect(() => {
    if (!open) {
      setSendBody('')
      setSelectedKey(null)
    }
  }, [open])

  useEffect(() => {
    if (!open) return
    try {
      const raw = localStorage.getItem(READ_STORAGE)
      if (raw) {
        const arr = JSON.parse(raw) as unknown
        if (Array.isArray(arr)) setReadIds(new Set(arr.filter((x): x is string => typeof x === 'string')))
      }
    } catch {
      /* ignore */
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

  const unifiedList = useMemo((): UnifiedItem[] => {
    const items: UnifiedItem[] = []
    for (const row of internalMergedSorted) {
      items.push({ kind: 'internal', row, sortKey: row.created_at || '' })
    }
    for (const row of crossRows) {
      items.push({ kind: 'cross', row, sortKey: row.created_at || '' })
    }
    items.sort((a, b) => b.sortKey.localeCompare(a.sortKey))
    return items
  }, [internalMergedSorted, crossRows])

  const markRead = useCallback((key: string) => {
    setReadIds((prev) => {
      if (prev.has(key)) return prev
      const next = new Set(prev)
      next.add(key)
      try {
        localStorage.setItem(READ_STORAGE, JSON.stringify([...next]))
      } catch {
        /* ignore */
      }
      return next
    })
  }, [])

  const selectItem = useCallback(
    (u: UnifiedItem) => {
      const k = itemKey(u)
      setSelectedKey(k)
      markRead(k)
    },
    [markRead]
  )

  useEffect(() => {
    if (!selectedKey) return
    const still = unifiedList.some((u) => itemKey(u) === selectedKey)
    if (!still) setSelectedKey(null)
  }, [unifiedList, selectedKey])

  const loadMoreInternal = useCallback(async () => {
    if (internalLoadingMore || internalLoading || !internalHasMore) return
    setInternalLoadingMore(true)
    try {
      await loadInternalPage(false)
    } finally {
      setInternalLoadingMore(false)
    }
  }, [internalLoadingMore, internalLoading, internalHasMore, loadInternalPage])

  const loadMoreCross = useCallback(async () => {
    if (crossLoadingMore || crossLoading || !crossHasMore) return
    setCrossLoadingMore(true)
    try {
      await loadCrossPage(false)
    } finally {
      setCrossLoadingMore(false)
    }
  }, [crossLoadingMore, crossLoading, crossHasMore, loadCrossPage])

  const listScrollRef = useRef<HTMLDivElement>(null)
  const handleListScroll = useCallback(() => {
    const el = listScrollRef.current
    if (!el) return
    if (el.scrollHeight - el.scrollTop - el.clientHeight > 80) return
    void loadMoreInternal()
    void loadMoreCross()
  }, [loadMoreInternal, loadMoreCross])

  const selectedItem = useMemo(() => {
    if (!selectedKey) return null
    return unifiedList.find((u) => itemKey(u) === selectedKey) ?? null
  }, [unifiedList, selectedKey])

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
      await loadCrossPage(true)
    } catch (e2) {
      setCrossErr(e2 instanceof Error ? e2.message : String(e2))
    } finally {
      setSending(false)
    }
  }

  const listLoading = internalLoading || crossLoading
  const listLoadingMore = internalLoadingMore || crossLoadingMore
  const listHasMore = internalHasMore || crossHasMore

  const listEmpty = !listLoading && unifiedList.length === 0

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center p-3 sm:p-4 bg-black/50"
      role="dialog"
      aria-modal="true"
      aria-labelledby="mailbox-title"
      onClick={() => onClose()}
    >
      <div
        className="bg-white rounded-xl shadow-2xl max-w-6xl w-full max-h-[92vh] flex flex-col border border-slate-200/90 overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        <header className="flex-shrink-0 px-4 sm:px-5 pt-4 pb-4 border-b border-slate-200 bg-gradient-to-b from-slate-50/90 to-white">
          <div className="flex items-start justify-between gap-3">
            <div>
              <h2 id="mailbox-title" className="text-lg font-semibold text-slate-900 tracking-tight">
                邮箱
              </h2>
              <p className="text-xs text-slate-500 mt-0.5">内部邮件与跨实例消息 · Esc 关闭</p>
            </div>
            <button
              type="button"
              onClick={() => onClose()}
              className="text-slate-400 hover:text-slate-700 p-1 rounded-md hover:bg-slate-100 text-xl leading-none"
              aria-label="关闭"
            >
              ×
            </button>
          </div>
        </header>

        <div className="flex flex-1 min-h-0 flex-col md:flex-row">
          <aside className="w-full md:w-[min(100%,380px)] md:flex-shrink-0 flex flex-col border-b md:border-b-0 md:border-r border-slate-200 bg-slate-50/50 min-h-[200px] md:min-h-0 md:max-w-[42%]">
            <div className="flex items-center justify-between gap-2 px-3 py-2 border-b border-slate-200/80 bg-white/80">
              <span className="text-xs text-slate-500">
                {listLoading ? '加载中…' : `共 ${unifiedList.length} 封`}
              </span>
              <button
                type="button"
                disabled={internalLoading || crossLoading}
                onClick={() => {
                  void loadInternalPage(true)
                  void loadCrossPage(true)
                }}
                className="text-xs px-2.5 py-1 rounded-md border border-slate-200 bg-white hover:bg-slate-50 disabled:opacity-50"
              >
                刷新
              </button>
            </div>
            {(internalErr || crossErr) && (
              <div className="mx-3 mt-2 rounded-md border border-rose-200 bg-rose-50 px-2.5 py-1.5 text-xs text-rose-800">
                {[internalErr, crossErr].filter(Boolean).join(' ')}
              </div>
            )}
            <div
              ref={listScrollRef}
              onScroll={handleListScroll}
              className="flex-1 overflow-y-auto min-h-0"
            >
              {listLoading && unifiedList.length === 0 ? (
                <p className="text-slate-500 text-sm py-16 text-center">加载邮件…</p>
              ) : listEmpty ? (
                <p className="text-slate-400 text-sm py-16 text-center">暂无邮件</p>
              ) : (
                <ul className="divide-y divide-slate-100">
                  {unifiedList.map((u) => {
                    const k = itemKey(u)
                    const isSel = selectedKey === k
                    const isRead = readIds.has(k)
                    if (u.kind === 'internal') {
                      const m = u.row
                      const inst = idToName.get(m.instance_id) ?? `#${m.instance_id}`
                      const subj = m.subject?.trim() || '（无主题）'
                      const prev = previewText(m.body || '')
                      return (
                        <li key={k}>
                          <button
                            type="button"
                            onClick={() => selectItem(u)}
                            className={`w-full text-left px-3 py-2.5 flex gap-2 hover:bg-slate-100/80 transition-colors ${
                              isSel ? 'bg-blue-50/90 border-l-[3px] border-l-blue-600 pl-[9px]' : 'border-l-[3px] border-l-transparent pl-[9px]'
                            }`}
                          >
                            <div className="flex-1 min-w-0">
                              <div className="flex items-baseline justify-between gap-2">
                                <span
                                  className={`text-sm truncate ${isRead ? 'text-slate-700' : 'text-slate-900 font-semibold'}`}
                                >
                                  {subj}
                                </span>
                                <span className="text-[11px] text-slate-400 flex-shrink-0 tabular-nums">
                                  {formatMailTime(m.created_at)}
                                </span>
                              </div>
                              <div className="text-xs text-slate-500 mt-0.5 truncate">
                                <span className="inline-flex items-center gap-1">
                                  <span className="text-[10px] uppercase tracking-wide text-violet-600 font-medium">内部</span>
                                  <span>{m.from_slug}</span>
                                  <span className="text-slate-300">→</span>
                                  <span>{m.to_slug}</span>
                                  <span className="text-slate-300">·</span>
                                  <span className="truncate">{inst}</span>
                                </span>
                              </div>
                              {prev ? (
                                <p className={`text-xs mt-1 line-clamp-2 ${isRead ? 'text-slate-500' : 'text-slate-600'}`}>
                                  {prev}
                                </p>
                              ) : null}
                            </div>
                          </button>
                        </li>
                      )
                    }
                    const m = u.row
                    const fromName = idToName.get(m.from_instance_id) ?? `#${m.from_instance_id}`
                    const toName = idToName.get(m.to_instance_id) ?? `#${m.to_instance_id}`
                    const prev = previewText(m.content || '')
                    return (
                      <li key={k}>
                        <button
                          type="button"
                          onClick={() => selectItem(u)}
                          className={`w-full text-left px-3 py-2.5 flex gap-2 hover:bg-slate-100/80 transition-colors ${
                            isSel ? 'bg-blue-50/90 border-l-[3px] border-l-blue-600 pl-[9px]' : 'border-l-[3px] border-l-transparent pl-[9px]'
                          }`}
                        >
                          <div className="flex-1 min-w-0">
                            <div className="flex items-baseline justify-between gap-2">
                              <span
                                className={`text-sm truncate ${isRead ? 'text-slate-700' : 'text-slate-900 font-semibold'}`}
                              >
                                {fromName} → {toName}
                              </span>
                              <span className="text-[11px] text-slate-400 flex-shrink-0 tabular-nums">
                                {formatMailTime(m.created_at)}
                              </span>
                            </div>
                            <div className="text-xs text-slate-500 mt-0.5 truncate">
                              <span className="inline-flex items-center gap-1">
                                <span className="text-[10px] uppercase tracking-wide text-indigo-600 font-medium">跨实例</span>
                                <span className="truncate">{fromName}</span>
                                <span className="text-slate-300">→</span>
                                <span className="truncate">{toName}</span>
                              </span>
                            </div>
                            {prev ? (
                              <p className={`text-xs mt-1 line-clamp-2 ${isRead ? 'text-slate-500' : 'text-slate-600'}`}>{prev}</p>
                            ) : null}
                          </div>
                        </button>
                      </li>
                    )
                  })}
                </ul>
              )}
              {listLoadingMore && listHasMore && unifiedList.length > 0 ? (
                <div className="py-3 text-center text-xs text-slate-500">加载更多…</div>
              ) : null}
            </div>
          </aside>

          <section className="flex-1 flex flex-col min-w-0 min-h-[240px] md:min-h-0 bg-white">
            {!selectedItem ? (
              <div className="flex-1 flex flex-col items-center justify-center text-slate-400 text-sm px-6 py-12">
                <div className="w-16 h-16 rounded-full bg-slate-100 flex items-center justify-center mb-3 text-2xl">✉</div>
                <p>从左侧列表选择一封邮件查看详情</p>
              </div>
            ) : selectedItem.kind === 'internal' ? (
              <div className="flex-1 flex flex-col min-h-0 overflow-hidden">
                <div className="flex-shrink-0 px-5 py-4 border-b border-slate-100 bg-white">
                  <div className="flex flex-wrap items-center gap-2 text-xs text-slate-500 mb-2">
                    <span className="px-2 py-0.5 rounded bg-violet-100 text-violet-800 font-medium">内部邮件</span>
                    <span>{formatMailTime(selectedItem.row.created_at)}</span>
                    <span className="font-mono text-[11px]">#{selectedItem.row.id}</span>
                  </div>
                  <h3 className="text-lg font-semibold text-slate-900 leading-snug">
                    {selectedItem.row.subject?.trim() || '（无主题）'}
                  </h3>
                  <div className="mt-3 space-y-1 text-sm">
                    <div className="flex flex-wrap gap-x-4 gap-y-1">
                      <span className="text-slate-500 w-12 flex-shrink-0">发件</span>
                      <span className="text-slate-900">{selectedItem.row.from_slug}</span>
                      <span className="text-slate-400">（{idToName.get(selectedItem.row.instance_id) ?? `#${selectedItem.row.instance_id}`}）</span>
                    </div>
                    <div className="flex flex-wrap gap-x-4 gap-y-1">
                      <span className="text-slate-500 w-12 flex-shrink-0">收件</span>
                      <span className="text-slate-900">{selectedItem.row.to_slug}</span>
                    </div>
                    {selectedItem.row.thread_id ? (
                      <div className="flex flex-wrap gap-x-2 text-xs text-slate-500 pt-1">
                        <span>会话</span>
                        <code className="bg-slate-100 px-1.5 py-0.5 rounded break-all">{selectedItem.row.thread_id}</code>
                      </div>
                    ) : null}
                    {selectedItem.row.in_reply_to != null ? (
                      <div className="text-xs text-slate-500">回复 #{selectedItem.row.in_reply_to}</div>
                    ) : null}
                  </div>
                </div>
                <div className="flex-1 overflow-y-auto px-5 py-4 text-sm text-slate-800 whitespace-pre-wrap break-words leading-relaxed">
                  {selectedItem.row.body || '—'}
                </div>
              </div>
            ) : (
              <div className="flex-1 flex flex-col min-h-0 overflow-hidden">
                <div className="flex-shrink-0 px-5 py-4 border-b border-slate-100">
                  <div className="flex flex-wrap items-center gap-2 text-xs text-slate-500 mb-2">
                    <span className="px-2 py-0.5 rounded bg-indigo-100 text-indigo-800 font-medium">跨实例</span>
                    <span>{formatMailTime(selectedItem.row.created_at)}</span>
                    <span className="font-mono text-[11px]">#{selectedItem.row.id}</span>
                  </div>
                  <div className="space-y-2 text-sm">
                    <div className="flex flex-wrap gap-x-4 gap-y-1">
                      <span className="text-slate-500 w-12 flex-shrink-0">发件</span>
                      <span className="font-medium text-slate-900">
                        {idToName.get(selectedItem.row.from_instance_id) ?? `#${selectedItem.row.from_instance_id}`}
                      </span>
                    </div>
                    <div className="flex flex-wrap gap-x-4 gap-y-1">
                      <span className="text-slate-500 w-12 flex-shrink-0">收件</span>
                      <span className="font-medium text-slate-900">
                        {idToName.get(selectedItem.row.to_instance_id) ?? `#${selectedItem.row.to_instance_id}`}
                      </span>
                    </div>
                  </div>
                </div>
                <div className="flex-1 overflow-y-auto px-5 py-4 text-sm text-slate-800 whitespace-pre-wrap break-words leading-relaxed">
                  {selectedItem.row.content || '—'}
                </div>
              </div>
            )}

            {instances.length >= 2 && (
              <form onSubmit={handleSendCross} className="flex-shrink-0 border-t border-slate-200 bg-slate-50/90 px-4 py-3 space-y-2">
                <p className="text-xs text-slate-500">发送跨实例消息（需在编排拓扑中已连线）</p>
                <div className="flex flex-col sm:flex-row gap-2 sm:items-center flex-wrap">
                  <label className="text-xs text-slate-600 flex items-center gap-1.5">
                    从
                    <select
                      value={sendFrom ?? ''}
                      onChange={(e) => setSendFrom(Number(e.target.value) || null)}
                      className="border border-slate-200 rounded-lg px-2 py-1 text-sm max-w-[160px] bg-white"
                    >
                      {instances.map((i) => (
                        <option key={i.id} value={i.id}>
                          {i.name}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label className="text-xs text-slate-600 flex items-center gap-1.5">
                    发往
                    <select
                      value={sendTo ?? ''}
                      onChange={(e) => setSendTo(Number(e.target.value) || null)}
                      className="border border-slate-200 rounded-lg px-2 py-1 text-sm max-w-[160px] bg-white"
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
                  className="w-full border border-slate-200 rounded-lg px-3 py-2 text-sm bg-white"
                />
                <div className="flex justify-end">
                  <button
                    type="submit"
                    disabled={sending || !sendBody.trim() || sendFrom == null || sendTo == null || sendFrom === sendTo}
                    className="px-4 py-2 text-sm bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 disabled:opacity-50"
                  >
                    {sending ? '发送中…' : '发送'}
                  </button>
                </div>
              </form>
            )}
          </section>
        </div>

        <footer className="flex-shrink-0 px-4 py-2.5 border-t border-slate-100 flex justify-end bg-slate-50/50 md:hidden">
          <button type="button" onClick={() => onClose()} className="px-4 py-2 text-sm bg-slate-800 text-white rounded-lg">
            关闭
          </button>
        </footer>
      </div>
    </div>
  )
}

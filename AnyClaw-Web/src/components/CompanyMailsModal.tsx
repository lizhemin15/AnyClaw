import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { getCollabMails, type InternalMailRow, type Instance } from '../api'

type Props = {
  open: boolean
  instances: Instance[]
  onClose: () => void
}

const PAGE = 80

export default function CompanyMailsModal({ open, instances, onClose }: Props) {
  const [rows, setRows] = useState<InternalMailRow[]>([])
  const [loading, setLoading] = useState(false)
  const [loadingMore, setLoadingMore] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [hasMore, setHasMore] = useState(false)
  const offsetRef = useRef<Record<number, number>>({})
  const exhaustedRef = useRef<Set<number>>(new Set())

  const idToName = useMemo(() => {
    const m = new Map<number, string>()
    for (const i of instances) m.set(i.id, i.name)
    return m
  }, [instances])

  const instanceIds = useMemo(() => instances.map((i) => i.id), [instances])

  const loadPage = useCallback(
    async (reset: boolean) => {
      if (instanceIds.length === 0) {
        setRows([])
        setHasMore(false)
        if (reset) setLoading(false)
        return
      }
      if (reset) {
        offsetRef.current = {}
        exhaustedRef.current = new Set()
        setLoading(true)
        setErr(null)
      }
      try {
        const offs = offsetRef.current
        const ids = instanceIds.filter((id) => !exhaustedRef.current.has(id))
        if (ids.length === 0) {
          setHasMore(false)
          if (reset) setRows([])
          return
        }
        const settled = await Promise.allSettled(
          ids.map((id) => getCollabMails(id, { limit: PAGE, offset: offs[id] ?? 0 }))
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
          if (list.length < PAGE || (typeof total === 'number' && offs[id] >= total)) {
            exhaustedRef.current.add(id)
          }
        }
        if (reset) {
          setRows(batch)
        } else {
          setRows((prev) => [...prev, ...batch])
        }
        const anyLeft = instanceIds.some((id) => !exhaustedRef.current.has(id))
        setHasMore(anyLeft)
        setErr(null)
      } catch (e) {
        setErr(e instanceof Error ? e.message : String(e))
        if (reset) setRows([])
      } finally {
        if (reset) setLoading(false)
      }
    },
    [instanceIds]
  )

  useEffect(() => {
    if (!open) return
    void loadPage(true)
  }, [open, loadPage])

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

  const loadMore = async () => {
    if (loadingMore || loading || !hasMore) return
    setLoadingMore(true)
    try {
      await loadPage(false)
    } finally {
      setLoadingMore(false)
    }
  }

  const mergedSorted = useMemo(() => {
    const list = [...rows]
    list.sort((a, b) => (b.created_at || '').localeCompare(a.created_at || ''))
    return list
  }, [rows])

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center p-4 bg-black/50"
      role="dialog"
      aria-modal="true"
      aria-labelledby="company-mails-title"
      onClick={() => onClose()}
    >
      <div
        className="bg-white rounded-2xl shadow-xl max-w-2xl w-full max-h-[90vh] flex flex-col border border-slate-200"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="px-5 pt-4 pb-2 border-b border-slate-100">
          <h2 id="company-mails-title" className="text-lg font-semibold text-slate-800">
            公司内部邮件
          </h2>
          <p className="text-xs text-slate-500 mt-2">汇总全部员工的内部邮件往来，按时间倒序，最新在上。按 Esc 关闭。</p>
        </div>
        <div className="px-5 py-4 flex-1 overflow-y-auto min-h-0">
          {err && (
            <div className="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-800 mb-3">{err}</div>
          )}
          <div className="flex flex-wrap items-center justify-between gap-2 mb-3">
            <p className="text-xs text-slate-500">共 {instances.length} 名员工（实例）</p>
            <button
              type="button"
              disabled={loading}
              onClick={() => void loadPage(true)}
              className="text-xs px-3 py-1.5 rounded-lg border border-slate-200 hover:bg-slate-50 disabled:opacity-50"
            >
              {loading ? '刷新中…' : '刷新'}
            </button>
          </div>
          {loading && mergedSorted.length === 0 ? (
            <p className="text-slate-500 text-sm py-12 text-center">加载邮件…</p>
          ) : mergedSorted.length === 0 ? (
            <p className="text-slate-400 text-sm py-8 text-center">暂无内部邮件</p>
          ) : (
            <div className="space-y-3">
              {mergedSorted.map((m) => (
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
          {hasMore && mergedSorted.length > 0 && (
            <button
              type="button"
              disabled={loadingMore}
              onClick={() => void loadMore()}
              className="w-full mt-3 py-2 rounded-lg border border-slate-200 text-sm text-slate-700 hover:bg-slate-50 disabled:opacity-50"
            >
              {loadingMore ? '加载中…' : '加载更多'}
            </button>
          )}
        </div>
        <div className="px-5 py-3 border-t border-slate-100 flex justify-end bg-slate-50/80 rounded-b-2xl">
          <button type="button" onClick={() => onClose()} className="px-4 py-2 text-sm bg-slate-800 text-white rounded-lg">
            关闭
          </button>
        </div>
      </div>
    </div>
  )
}

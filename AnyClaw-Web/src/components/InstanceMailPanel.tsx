import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  getCollabInstanceMails,
  postCollabInstanceMail,
  broadcastCollabEvent,
  type Instance,
  type UserInstanceMessageRow,
} from '../api'

type Props = {
  open: boolean
  instances: Instance[]
  onClose: () => void
}

const PAGE = 120

function mergeDedupe(rows: UserInstanceMessageRow[][]): UserInstanceMessageRow[] {
  const byId = new Map<number, UserInstanceMessageRow>()
  for (const batch of rows) {
    for (const m of batch) {
      if (!byId.has(m.id)) byId.set(m.id, m)
    }
  }
  return [...byId.values()].sort((a, b) => (b.created_at || '').localeCompare(a.created_at || ''))
}

export default function InstanceMailPanel({ open, instances, onClose }: Props) {
  const [rows, setRows] = useState<UserInstanceMessageRow[]>([])
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState<string | null>(null)
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

  const loadAll = useCallback(async () => {
    if (instanceIds.length === 0) {
      setRows([])
      return
    }
    setLoading(true)
    setErr(null)
    try {
      const settled = await Promise.allSettled(
        instanceIds.map((id) => getCollabInstanceMails(id, { limit: PAGE, offset: 0 }))
      )
      const batches: UserInstanceMessageRow[][] = []
      for (let i = 0; i < settled.length; i++) {
        const r = settled[i]
        if (r.status === 'fulfilled') batches.push(r.value.messages || [])
      }
      setRows(mergeDedupe(batches))
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
      setRows([])
    } finally {
      setLoading(false)
    }
  }, [instanceIds])

  useEffect(() => {
    if (!open) return
    void loadAll()
  }, [open, loadAll])

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

  const peerActivity = useMemo(() => {
    const m = new Map<number, string>()
    for (const row of rows) {
      for (const pid of [row.from_instance_id, row.to_instance_id]) {
        const t = row.created_at || ''
        const prev = m.get(pid)
        if (!prev || t > prev) m.set(pid, t)
      }
    }
    return m
  }, [rows])

  const peerIdsSorted = useMemo(() => {
    const ids = [...peerActivity.keys()]
    ids.sort((a, b) => (peerActivity.get(b) || '').localeCompare(peerActivity.get(a) || ''))
    return ids
  }, [peerActivity])

  const filtered = useMemo(() => {
    if (peerFilter == null) return rows
    return rows.filter((m) => m.from_instance_id === peerFilter || m.to_instance_id === peerFilter)
  }, [rows, peerFilter])

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault()
    if (sending || sendFrom == null || sendTo == null || sendFrom === sendTo) return
    const content = sendBody.trim()
    if (!content) return
    setSending(true)
    setErr(null)
    try {
      await postCollabInstanceMail(sendFrom, { to_instance_id: sendTo, content })
      broadcastCollabEvent('instance_mail', sendFrom)
      broadcastCollabEvent('instance_mail', sendTo)
      setSendBody('')
      await loadAll()
    } catch (e2) {
      setErr(e2 instanceof Error ? e2.message : String(e2))
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
      aria-labelledby="instance-mail-title"
      onClick={() => onClose()}
    >
      <div
        className="bg-white rounded-2xl shadow-xl max-w-3xl w-full max-h-[90vh] flex flex-col border border-slate-200"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="px-5 pt-4 pb-2 border-b border-slate-100">
          <h2 id="instance-mail-title" className="text-lg font-semibold text-slate-800">
            跨实例信箱
          </h2>
          <p className="text-xs text-slate-500 mt-2">查看账号下实例之间的跨实例消息往来；按 Esc 关闭。</p>
        </div>
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
            {err && (
              <div className="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-800 mb-3">{err}</div>
            )}
            <div className="flex flex-wrap items-center justify-between gap-2 mb-3">
              <p className="text-xs text-slate-500">
                {peerFilter == null ? `共 ${filtered.length} 条` : `与「${idToName.get(peerFilter) ?? peerFilter}」相关 ${filtered.length} 条`}
              </p>
              <button
                type="button"
                disabled={loading}
                onClick={() => void loadAll()}
                className="text-xs px-3 py-1.5 rounded-lg border border-slate-200 hover:bg-slate-50 disabled:opacity-50"
              >
                {loading ? '刷新中…' : '刷新'}
              </button>
            </div>
            {loading && filtered.length === 0 ? (
              <p className="text-slate-500 text-sm py-12 text-center">加载中…</p>
            ) : filtered.length === 0 ? (
              <p className="text-slate-400 text-sm py-8 text-center">暂无跨实例消息</p>
            ) : (
              <div className="space-y-3">
                {filtered.map((m) => {
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
          <form onSubmit={handleSend} className="px-5 py-3 border-t border-slate-100 bg-slate-50/80 space-y-2 rounded-b-2xl">
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
        <div className="px-5 py-3 border-t border-slate-100 flex justify-end bg-white rounded-b-2xl">
          <button type="button" onClick={() => onClose()} className="px-4 py-2 text-sm bg-slate-800 text-white rounded-lg">
            关闭
          </button>
        </div>
      </div>
    </div>
  )
}

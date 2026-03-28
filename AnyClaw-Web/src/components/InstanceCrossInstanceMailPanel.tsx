import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  ANYCLAW_COLLAB_BROADCAST,
  broadcastCollabEvent,
  getCollabInstanceMails,
  getCollabInstanceTopology,
  getInstances,
  postCollabInstanceMail,
  type CollabApiError,
  type CollabLimits,
  type Instance,
  type UserInstanceMessageRow,
} from '../api'

function neighborInstanceIds(edges: [number, number][], selfId: number): number[] {
  const s = new Set<number>()
  for (const [a, b] of edges) {
    if (a === selfId) s.add(b)
    else if (b === selfId) s.add(a)
  }
  return [...s].sort((x, y) => x - y)
}

export type InstanceCrossInstanceMailPanelProps = {
  instanceId: number
  /** 与 Home 编排内嵌等布局配合 */
  className?: string
}

export default function InstanceCrossInstanceMailPanel({
  instanceId,
  className = '',
}: InstanceCrossInstanceMailPanelProps) {
  const [limits, setLimits] = useState<CollabLimits | null>(null)
  const [instances, setInstances] = useState<Instance[]>([])
  const [neighborIds, setNeighborIds] = useState<number[]>([])
  const [messages, setMessages] = useState<UserInstanceMessageRow[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [sending, setSending] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [peerFilter, setPeerFilter] = useState<number | ''>('')
  const [toId, setToId] = useState<number | ''>('')
  const [draft, setDraft] = useState('')
  const nextOffRef = useRef(0)
  const [loadingMore, setLoadingMore] = useState(false)
  const [hasMore, setHasMore] = useState(false)

  const instanceIdRef = useRef(instanceId)
  instanceIdRef.current = instanceId

  const maxList = limits?.max_instance_message_list_limit ?? 500
  const maxOff = limits?.max_instance_message_list_offset ?? 500_000
  const pageSize = Math.min(100, Math.max(1, maxList))
  const maxBodyKb = limits?.max_instance_message_body_kb ?? 256

  const nameById = useMemo(() => {
    const m = new Map<number, string>()
    for (const i of instances) m.set(i.id, i.name || `#${i.id}`)
    return m
  }, [instances])

  const loadMeta = useCallback(async () => {
    const expected = instanceId
    const [instList, topo] = await Promise.all([getInstances(), getCollabInstanceTopology(expected)])
    if (instanceIdRef.current !== expected) return
    setInstances(instList)
    setNeighborIds(neighborInstanceIds(topo.edges || [], expected))
    if (topo.limits) setLimits((prev) => ({ ...prev, ...topo.limits }))
  }, [instanceId])

  const loadMessages = useCallback(
    async (append: boolean) => {
      const expected = instanceId
      const off = append ? nextOffRef.current : 0
      if (off > maxOff) {
        setErr(`列表 offset 超过上限（${maxOff}）`)
        return
      }
      if (!append) {
        setLoading(true)
        setErr(null)
      }
      try {
        const peer = typeof peerFilter === 'number' && peerFilter > 0 ? peerFilter : undefined
        const { messages: list, total: t, limits: lm } = await getCollabInstanceMails(expected, {
          limit: pageSize,
          offset: off,
          peer,
        })
        if (instanceIdRef.current !== expected) return
        if (lm) setLimits(lm)
        const batch = list || []
        const newOff = off + batch.length
        if (append) {
          setMessages((prev) => [...prev, ...batch])
        } else {
          setMessages(batch)
        }
        nextOffRef.current = newOff
        const totalN = typeof t === 'number' && Number.isFinite(t) ? t : 0
        setTotal(totalN)
        setHasMore(newOff < totalN && newOff <= maxOff)
        setErr(null)
      } catch (e) {
        if (instanceIdRef.current !== expected) return
        const lim = (e as CollabApiError).collabLimits
        if (lim) setLimits(lim)
        setErr(e instanceof Error ? e.message : String(e))
        if (!append) {
          setMessages([])
          nextOffRef.current = 0
          setHasMore(false)
        }
      } finally {
        if (instanceIdRef.current === expected && !append) setLoading(false)
      }
    },
    [instanceId, maxOff, pageSize, peerFilter]
  )

  useEffect(() => {
    void loadMeta()
  }, [loadMeta])

  useEffect(() => {
    nextOffRef.current = 0
    void loadMessages(false)
  }, [instanceId, peerFilter, loadMessages])

  useEffect(() => {
    if (typeof BroadcastChannel === 'undefined') return
    let bc: BroadcastChannel | null = null
    try {
      bc = new BroadcastChannel(ANYCLAW_COLLAB_BROADCAST)
      bc.onmessage = (ev: MessageEvent) => {
        const d = ev.data as { kind?: string; instanceId?: number }
        if (d.kind !== 'instance_mail' || d.instanceId !== instanceId) return
        void loadMessages(false)
      }
    } catch {
      /* noop */
    }
    return () => bc?.close()
  }, [instanceId, loadMessages])

  const send = async () => {
    const tid = typeof toId === 'number' ? toId : 0
    const text = draft.trim()
    if (tid < 1) {
      setErr('请选择接收实例')
      return
    }
    if (!text) {
      setErr('请输入内容')
      return
    }
    if (!neighborIds.includes(tid)) {
      setErr('仅可向编排拓扑中已连线的实例发送')
      return
    }
    setSending(true)
    setErr(null)
    try {
      await postCollabInstanceMail(instanceId, { to_instance_id: tid, content: draft })
      setDraft('')
      broadcastCollabEvent('instance_mail', instanceId)
      broadcastCollabEvent('instance_mail', tid)
      await loadMessages(false)
    } catch (e) {
      const lim = (e as CollabApiError).collabLimits
      if (lim) setLimits(lim)
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setSending(false)
    }
  }

  const loadMore = async () => {
    if (loadingMore || !hasMore) return
    setLoadingMore(true)
    try {
      await loadMessages(true)
    } finally {
      setLoadingMore(false)
    }
  }

  return (
    <div className={`rounded-2xl border border-violet-200/80 bg-white p-4 space-y-4 ${className}`}>
      <div>
        <h3 className="text-sm font-medium text-slate-800">跨实例消息</h3>
        <p className="text-xs text-slate-500 mt-1">
          仅可与账号编排拓扑中已连线的实例通信；正文上限约 {maxBodyKb} KB。
        </p>
      </div>

      {err && (
        <div className="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-800">{err}</div>
      )}

      <div className="flex flex-wrap gap-2 items-end">
        <label className="flex flex-col gap-1 text-xs text-slate-600">
          发往实例
          <select
            className="border border-slate-200 rounded-lg px-2 py-1.5 text-sm min-w-[160px]"
            value={toId === '' ? '' : String(toId)}
            onChange={(e) => {
              const v = e.target.value
              setToId(v === '' ? '' : parseInt(v, 10))
            }}
          >
            <option value="">选择已连线实例…</option>
            {neighborIds.map((id) => (
              <option key={id} value={id}>
                {nameById.get(id) ?? `#${id}`} (#{id})
              </option>
            ))}
          </select>
        </label>
        <button
          type="button"
          disabled={sending || neighborIds.length === 0}
          onClick={() => void send()}
          className="px-3 py-1.5 rounded-lg bg-violet-600 text-white text-sm hover:bg-violet-700 disabled:opacity-50"
        >
          {sending ? '发送中…' : '发送'}
        </button>
      </div>
      <textarea
        className="w-full min-h-[88px] border border-slate-200 rounded-xl px-3 py-2 text-sm"
        placeholder="输入要发送到对方实例的消息…"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
      />

      <div className="border-t border-slate-100 pt-3 space-y-2">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="text-xs text-slate-500">
            往来记录 {total > 0 ? `（${total} 条）` : ''}
          </span>
          <div className="flex flex-wrap gap-2 items-center">
            <label className="text-xs text-slate-600 flex items-center gap-1">
              筛选会话
              <select
                className="border border-slate-200 rounded-lg px-2 py-1 text-xs"
                value={peerFilter === '' ? '' : String(peerFilter)}
                onChange={(e) => {
                  const v = e.target.value
                  setPeerFilter(v === '' ? '' : parseInt(v, 10))
                }}
              >
                <option value="">全部</option>
                {neighborIds.map((id) => (
                  <option key={id} value={id}>
                    与 {nameById.get(id) ?? id}
                  </option>
                ))}
              </select>
            </label>
            <button
              type="button"
              disabled={loading}
              onClick={() => void loadMessages(false)}
              className="text-xs px-2 py-1 rounded border border-slate-200 hover:bg-slate-50 disabled:opacity-50"
            >
              {loading ? '刷新中…' : '刷新'}
            </button>
          </div>
        </div>
        {loading && messages.length === 0 ? (
          <p className="text-slate-500 text-sm py-6 text-center">加载中…</p>
        ) : messages.length === 0 ? (
          <p className="text-slate-400 text-sm py-6 text-center">暂无跨实例消息</p>
        ) : (
          <div className="space-y-2 max-h-[320px] overflow-y-auto">
            {messages.map((m) => {
              const out = m.from_instance_id === instanceId
              const peer = out ? m.to_instance_id : m.from_instance_id
              return (
                <div
                  key={m.id}
                  className={`border rounded-xl p-2.5 text-xs ${
                    out ? 'border-violet-200 bg-violet-50/50' : 'border-slate-200 bg-slate-50/50'
                  }`}
                >
                  <div className="flex flex-wrap gap-x-2 gap-y-0.5 text-slate-500 mb-1">
                    <span className="font-mono">#{m.id}</span>
                    <span>{m.created_at}</span>
                    <span>{out ? '发出 →' : '← 收到'}</span>
                    <span>
                      {nameById.get(peer) ?? `#${peer}`} (#{peer})
                    </span>
                  </div>
                  <div className="text-slate-800 whitespace-pre-wrap break-words">{m.content}</div>
                </div>
              )
            })}
          </div>
        )}
        {hasMore && messages.length > 0 && (
          <button
            type="button"
            disabled={loadingMore}
            onClick={() => void loadMore()}
            className="w-full py-2 rounded-lg border border-slate-200 text-sm text-slate-700 hover:bg-slate-50 disabled:opacity-50"
          >
            {loadingMore ? '加载中…' : '更早的消息'}
          </button>
        )}
      </div>
    </div>
  )
}

import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  ANYCLAW_COLLAB_BROADCAST,
  getCollabMails,
  type CollabApiError,
  type CollabLimits,
  type InternalMailRow,
} from '../api'
import CollabTopologyPanel from './CollabTopologyPanel'

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
  /** 协作名单变更后传入以刷新拓扑节点（与 InstanceCollab 保存员工一致） */
  rosterRevision?: number
}

export default function HomeCollabOrchestrateModal({
  open,
  instanceId,
  instanceName,
  onClose,
  onSaved,
  initialCollabTab = 'topo',
  variant = 'modal',
  rosterRevision = 0,
}: HomeCollabOrchestrateModalProps) {
  const [collabTab, setCollabTab] = useState<'topo' | 'mails'>(initialCollabTab)
  const [limits, setLimits] = useState<CollabLimits | null>(null)
  const [mails, setMails] = useState<InternalMailRow[]>([])
  const [mailLoading, setMailLoading] = useState(false)
  const [mailLoadingMore, setMailLoadingMore] = useState(false)
  const [mailErr, setMailErr] = useState<string | null>(null)
  const [mailHasMore, setMailHasMore] = useState(false)
  const mailNextOffsetRef = useRef(0)
  const panelRef = useRef<HTMLDivElement>(null)
  const instanceIdRef = useRef(instanceId)
  instanceIdRef.current = instanceId

  useEffect(() => {
    if (!open) return
    if (variant === 'inline') {
      setCollabTab('topo')
      return
    }
    if (initialCollabTab === 'mails') setCollabTab('mails')
    else setCollabTab('topo')
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

  const [topoDirty, setTopoDirty] = useState(false)

  const requestClose = useCallback(() => {
    if (topoDirty) {
      if (!confirm('有未保存的拓扑变更，确定关闭？')) return
    }
    onClose()
  }, [topoDirty, onClose])

  const collabTabRef = useRef(collabTab)
  collabTabRef.current = collabTab

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
        }
      }
    } catch {
      /* noop */
    }
    return () => bc?.close()
  }, [open, instanceId, loadMails])

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
    if (!open || variant === 'inline') return
    const t = window.setTimeout(() => {
      panelRef.current?.focus()
    }, 50)
    return () => clearTimeout(t)
  }, [open, variant])

  const orchDescId = 'home-orch-desc'

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
          </div>
          {variant !== 'inline' && (
            <div className="flex gap-1 p-1 bg-slate-100 rounded-xl w-fit mt-3 flex-wrap">
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
              <>拓扑图展示账号下全部招募实例，可连线编排（名单与连线由 API 同步）。</>
            ) : collabTab === 'topo' ? (
              <>编排标签下可拖拽或点击连线；员工列表来自 API。按 Esc 关闭。</>
            ) : (
              <>员工之间往来的内部邮件，按时间倒序，最新在最上。按 Esc 关闭。</>
            )}
          </p>
        </div>

        <div className="px-5 py-4 flex-1 overflow-y-auto min-h-0 space-y-4">
          {variant === 'inline' ? (
            <div className="space-y-4">
              <CollabTopologyPanel
                key={`${instanceId}-${rosterRevision}`}
                instanceId={instanceId}
                nodeSource="instances"
                rosterRevision={rosterRevision}
                onDirtyChange={setTopoDirty}
                onTopologySaved={() => {
                  onSaved?.()
                }}
              />
            </div>
          ) : (
            <>
              <div
                className={collabTab === 'mails' ? 'hidden' : ''}
                aria-hidden={collabTab === 'mails'}
              >
                <CollabTopologyPanel
                  key={`${instanceId}-${rosterRevision}`}
                  instanceId={instanceId}
                  nodeSource="instances"
                  rosterRevision={rosterRevision}
                  onDirtyChange={setTopoDirty}
                  onTopologySaved={() => {
                    onSaved?.()
                    onClose()
                  }}
                />
              </div>
              {collabTab === 'mails' && (
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
              )}
            </>
          )}
        </div>

        <div className="px-5 py-3 border-t border-slate-100 flex flex-wrap gap-2 justify-end items-center bg-slate-50/80 rounded-b-2xl">
          <Link
            to={`/instances/${instanceId}/collab`}
            onClick={(e) => {
              if (topoDirty) {
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
        </div>
      </div>
    </div>
  )
}

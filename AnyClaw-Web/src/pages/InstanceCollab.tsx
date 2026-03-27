import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { SafeLink } from '../components/SafeLink'
import {
  broadcastCollabEvent,
  getInstance,
  getCollabAgents,
  putCollabAgents,
  getCollabTopology,
  putCollabTopology,
  getCollabMails,
  postCollabResolve,
  type CollabApiError,
  ANYCLAW_COLLAB_BROADCAST,
  type CollabAgent,
  type CollabLimits,
  type InternalMailRow,
} from '../api'

type Tab = 'agents' | 'topology' | 'mails'

export default function InstanceCollab() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const instanceId = parseInt(id || '', 10)

  const [tab, setTab] = useState<Tab>('agents')
  const [instanceName, setInstanceName] = useState('')
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const [agentRows, setAgentRows] = useState<{ agent_slug: string; display_name: string }[]>([])
  const [edges, setEdges] = useState<[string, string][]>([])
  const [topoVersion, setTopoVersion] = useState(0)
  const [mails, setMails] = useState<InternalMailRow[]>([])
  /** 输入框即时值 */
  const [mailThreadFilterInput, setMailThreadFilterInput] = useState('')
  /** 防抖后用于请求（避免每键一次 API） */
  const [mailThreadQuery, setMailThreadQuery] = useState('')
  /** 当前列表实际使用的 thread 筛选（空表示未按 thread 筛） */
  const [mailActiveThread, setMailActiveThread] = useState('')
  const [mailHasMore, setMailHasMore] = useState(false)
  const [mailLoadingMore, setMailLoadingMore] = useState(false)
  const [mailTotal, setMailTotal] = useState<number | null>(null)
  const [mailListLoading, setMailListLoading] = useState(false)
  /** 仅邮件列表/加载更多/刷新失败，与员工、拓扑错误分离 */
  const [mailListErr, setMailListErr] = useState<string | null>(null)
  const mailNextOffsetRef = useRef(0)
  const [edgeA, setEdgeA] = useState('')
  const [edgeB, setEdgeB] = useState('')
  const [resolveName, setResolveName] = useState('')
  const [resolveOut, setResolveOut] = useState<string | null>(null)
  const [copiedThreadTip, setCopiedThreadTip] = useState<string | null>(null)
  const copyTipTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [collabLimits, setCollabLimits] = useState<CollabLimits | null>(null)

  const slugOptions = agentRows.map((r) => r.agent_slug).filter(Boolean)
  const maxAgentsCap = collabLimits?.max_agents ?? 64
  const maxEdgesCap = collabLimits?.max_edges ?? 4096
  const maxThreadFilterRunes = collabLimits?.max_thread_id_runes ?? 64
  const maxSlugRunes = collabLimits?.max_agent_slug_runes ?? 128
  const maxDisplayRunes = collabLimits?.max_agent_display_name_runes ?? 255
  const maxMailListLimit = collabLimits?.max_internal_mail_list_limit ?? 500
  const maxMailListOffsetCap = collabLimits?.max_internal_mail_list_offset ?? 500_000
  const mailPageSize = Math.min(100, Math.max(1, maxMailListLimit))
  const canAddAgentRow = agentRows.length < maxAgentsCap
  const atEdgeLimit = edges.length >= maxEdgesCap

  const validAgentRowCount = useMemo(
    () => agentRows.filter((r) => r.agent_slug.trim() !== '' && r.display_name.trim() !== '').length,
    [agentRows]
  )

  const copyThreadId = (tid: string) => {
    if (!tid || tid === '—') return
    if (copyTipTimerRef.current) clearTimeout(copyTipTimerRef.current)
    void navigator.clipboard.writeText(tid).then(() => {
      setCopiedThreadTip(tid)
      copyTipTimerRef.current = setTimeout(() => {
        setCopiedThreadTip((cur) => (cur === tid ? null : cur))
        copyTipTimerRef.current = null
      }, 2000)
    })
  }

  useEffect(() => {
    return () => {
      if (copyTipTimerRef.current) clearTimeout(copyTipTimerRef.current)
    }
  }, [])

  const loadAgents = useCallback(async () => {
    const { agents, limits } = await getCollabAgents(instanceId)
    setCollabLimits(limits ?? null)
    setAgentRows(
      agents.length
        ? agents.map((a: CollabAgent) => ({ agent_slug: a.agent_slug, display_name: a.display_name }))
        : [{ agent_slug: 'main', display_name: '主理' }]
    )
  }, [instanceId])

  const loadTopology = useCallback(async () => {
    const t = await getCollabTopology(instanceId)
    setEdges(t.edges || [])
    setTopoVersion(t.version ?? 0)
    if (t.limits) setCollabLimits(t.limits)
  }, [instanceId])

  const loadMails = useCallback(
    async (opts?: { append?: boolean; thread?: string }) => {
      const append = opts?.append === true
      if (!append) {
        setMailListLoading(true)
        setMailListErr(null)
      }
      try {
        const threadRaw = opts?.thread !== undefined ? opts.thread : mailThreadQuery
        const thread = threadRaw.trim()
        if (thread.length > 0) {
          const threadRunes = [...thread].length
          if (threadRunes > maxThreadFilterRunes) {
            setMailListErr(`thread_id 过长（最多 ${maxThreadFilterRunes} 字）`)
            if (!append) {
              setMails([])
              setMailTotal(null)
              setMailHasMore(false)
              mailNextOffsetRef.current = 0
              setMailActiveThread(thread)
            }
            return
          }
        }
        const off = append ? mailNextOffsetRef.current : 0
        if (off > maxMailListOffsetCap) {
          setMailListErr(
            `邮件列表 offset 超过上限（最多 ${maxMailListOffsetCap}，请用 thread_id 筛选或联系管理员调大配额）。`
          )
          if (!append) {
            setMails([])
            setMailTotal(null)
            setMailHasMore(false)
            mailNextOffsetRef.current = 0
            setMailActiveThread(thread)
          }
          return
        }
        const { mails: list, total, limits: mailLimits } = await getCollabMails(instanceId, {
          thread_id: thread || undefined,
          limit: mailPageSize,
          offset: off,
        })
        if (mailLimits) setCollabLimits(mailLimits)
        const batch = list || []
        const totalN = typeof total === 'number' && Number.isFinite(total) ? total : null
        setMailTotal(totalN)
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
          setMailHasMore(batch.length >= mailPageSize)
        }
        if (!append) {
          setMailActiveThread(thread)
        }
        setMailListErr(null)
      } catch (e) {
        const lim = (e as CollabApiError).collabLimits
        if (lim) setCollabLimits(lim)
        setMailListErr(e instanceof Error ? e.message : String(e))
      } finally {
        if (!append) {
          setMailListLoading(false)
        }
      }
    },
    [instanceId, mailThreadQuery, maxThreadFilterRunes, mailPageSize, maxMailListOffsetCap]
  )

  useEffect(() => {
    const id = window.setTimeout(() => setMailThreadQuery(mailThreadFilterInput.trim()), 400)
    return () => window.clearTimeout(id)
  }, [mailThreadFilterInput])

  const loadMoreMails = async () => {
    if (!mailHasMore || mailLoadingMore) return
    setMailLoadingMore(true)
    try {
      await loadMails({ append: true })
    } finally {
      setMailLoadingMore(false)
    }
  }

  const mailThreads = useMemo(() => {
    const byThread = new Map<string, InternalMailRow[]>()
    for (const m of mails) {
      const tid = (m.thread_id || '').trim() || '—'
      const arr = byThread.get(tid) || []
      arr.push(m)
      byThread.set(tid, arr)
    }
    const entries = [...byThread.entries()].map(([threadId, items]) => {
      const sorted = [...items].sort((a, b) => a.id - b.id)
      const last = sorted[sorted.length - 1]
      return { threadId, mails: sorted, lastId: last?.id ?? 0, lastAt: last?.created_at ?? '' }
    })
    entries.sort((a, b) => b.lastId - a.lastId)
    return entries
  }, [mails])

  useEffect(() => {
    if (isNaN(instanceId)) return
    let cancelled = false
    ;(async () => {
      setLoading(true)
      setErr(null)
      try {
        const inst = await getInstance(instanceId)
        if (cancelled) return
        setInstanceName(inst.name)
        await loadAgents()
        await loadTopology()
      } catch (e) {
        if (!cancelled) setErr(e instanceof Error ? e.message : String(e))
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [instanceId, loadAgents, loadTopology])

  useEffect(() => {
    if (isNaN(instanceId) || tab !== 'mails') return
    void loadMails()
  }, [instanceId, tab, loadMails])

  useEffect(() => {
    if (isNaN(instanceId) || typeof BroadcastChannel === 'undefined') return
    let bc: BroadcastChannel | null = null
    try {
      bc = new BroadcastChannel(ANYCLAW_COLLAB_BROADCAST)
      bc.onmessage = (ev: MessageEvent) => {
        const d = ev.data as { kind?: string; instanceId?: number }
        if (d.instanceId !== instanceId) return
        if (d.kind === 'internal_mail' && tab === 'mails') {
          void loadMails()
        }
        if (d.kind === 'topology') {
          void loadAgents()
          void loadTopology()
        }
      }
    } catch {
      /* noop */
    }
    return () => {
      bc?.close()
    }
  }, [instanceId, tab, loadMails, loadAgents, loadTopology])

  const saveAgents = async () => {
    setSaving(true)
    setErr(null)
    try {
      const cleaned = agentRows
        .map((r) => ({
          agent_slug: r.agent_slug.trim(),
          display_name: r.display_name.trim(),
        }))
        .filter((r) => r.agent_slug && r.display_name)
      if (cleaned.length === 0) {
        setErr('至少保留一名有效员工（须同时填写 agent_slug 与展示名）。')
        return
      }
      if (cleaned.length > maxAgentsCap) {
        setErr(`有效员工不得超过 ${maxAgentsCap} 名。`)
        return
      }
      const seenSlug = new Set<string>()
      const seenName = new Set<string>()
      for (const r of cleaned) {
        if (seenSlug.has(r.agent_slug)) {
          setErr(`列表中存在重复的 agent_slug：${r.agent_slug}`)
          return
        }
        seenSlug.add(r.agent_slug)
        if (seenName.has(r.display_name)) {
          setErr(`列表中存在重复的展示名：${r.display_name}`)
          return
        }
        seenName.add(r.display_name)
      }
      for (const r of cleaned) {
        if ([...r.agent_slug].length > maxSlugRunes) {
          setErr(`agent_slug 过长（最多 ${maxSlugRunes} 字）`)
          return
        }
        if ([...r.display_name].length > maxDisplayRunes) {
          setErr(`展示名过长（最多 ${maxDisplayRunes} 字）`)
          return
        }
      }
      const putA = await putCollabAgents(instanceId, cleaned)
      if (putA.limits) setCollabLimits(putA.limits)
      broadcastCollabEvent('topology', instanceId)
      await loadAgents()
      await loadTopology()
    } catch (e) {
      const lim = (e as CollabApiError).collabLimits
      if (lim) setCollabLimits(lim)
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  const saveTopology = async () => {
    setSaving(true)
    setErr(null)
    try {
      if (edges.length > maxEdgesCap) {
        setErr(`无向边不得超过 ${maxEdgesCap} 条。`)
        return
      }
      const formReadySlugs = new Set(
        agentRows
          .filter((r) => r.agent_slug.trim() !== '' && r.display_name.trim() !== '')
          .map((r) => r.agent_slug.trim())
      )
      if (edges.length > 0 && formReadySlugs.size === 0) {
        setErr('保存连线前请至少保留一行完整员工（agent_slug 与展示名均非空），建议先保存员工列表。')
        return
      }
      for (const pair of edges) {
        const a = pair[0].trim()
        const b = pair[1].trim()
        if (!a || !b) {
          setErr('每条连线的两端均须为有效的 agent_slug。')
          return
        }
        if ([...a].length > maxSlugRunes || [...b].length > maxSlugRunes) {
          setErr(`连线中的 agent_slug 过长（最多 ${maxSlugRunes} 字）`)
          return
        }
        const lo = a < b ? a : b
        const hi = a < b ? b : a
        if (lo === hi) {
          setErr('不能将员工与自身连线。')
          return
        }
        if (!formReadySlugs.has(lo)) {
          setErr(
            `agent_slug「${lo}」须对应表格中已填完整的员工；若刚新增该员工，请先保存员工再保存拓扑。`
          )
          return
        }
        if (!formReadySlugs.has(hi)) {
          setErr(
            `agent_slug「${hi}」须对应表格中已填完整的员工；若刚新增该员工，请先保存员工再保存拓扑。`
          )
          return
        }
      }
      const putT = await putCollabTopology(instanceId, edges)
      if (putT.limits) setCollabLimits(putT.limits)
      broadcastCollabEvent('topology', instanceId)
      await loadTopology()
    } catch (e) {
      const lim = (e as CollabApiError).collabLimits
      if (lim) setCollabLimits(lim)
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  const addEdge = () => {
    if (edges.length >= maxEdgesCap) return
    const a = edgeA.trim()
    const b = edgeB.trim()
    if (!a || !b || a === b) return
    const lo = a < b ? a : b
    const hi = a < b ? b : a
    const key = `${lo}\0${hi}`
    const exists = edges.some(([x, y]) => {
      const L = x < y ? x : y
      const H = x < y ? y : x
      return `${L}\0${H}` === key
    })
    if (exists) return
    setEdges([...edges, [lo, hi]])
  }

  const removeEdge = (i: number) => {
    setEdges(edges.filter((_, j) => j !== i))
  }

  const tryResolve = async () => {
    setResolveOut(null)
    setErr(null)
    try {
      const r = await postCollabResolve(instanceId, resolveName.trim())
      if (r.limits) setCollabLimits(r.limits)
      if (r.ok && r.agent_slug) setResolveOut(`→ agent_slug: ${r.agent_slug}`)
      else if (r.ambiguous?.length) setResolveOut(`歧义: ${r.ambiguous.join('、')}`)
      else if (r.not_found) setResolveOut('未找到')
      else setResolveOut(JSON.stringify(r))
    } catch (e) {
      const lim = (e as CollabApiError).collabLimits
      if (lim) setCollabLimits(lim)
      setErr(e instanceof Error ? e.message : String(e))
    }
  }

  if (isNaN(instanceId)) {
    return <p className="text-red-600">无效的实例 ID</p>
  }

  return (
    <div className="max-w-4xl mx-auto space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <SafeLink
          to={`/instances/${instanceId}`}
          className="text-sm text-indigo-600 hover:text-indigo-800"
        >
          ← 返回对话
        </SafeLink>
        <span className="text-slate-300">|</span>
        <button
          type="button"
          onClick={() => navigate('/', { state: { orchestrateInstanceId: instanceId } })}
          className="text-sm text-violet-600 hover:text-violet-800"
        >
          首页编排
        </button>
        <span className="text-slate-300">|</span>
        <h1 className="text-lg font-semibold text-slate-800">协作 · {instanceName || `#${instanceId}`}</h1>
      </div>
      <p className="text-sm text-slate-500">
        员工展示名在同一账号下唯一；拓扑为无向边，仅邻居可发内部邮件。保存员工/拓扑或容器发内部邮件后，实例会通过 WebSocket 通知容器；若同实例的对话页保持连接，本页会跨标签自动刷新邮件与拓扑。
      </p>

      <div className="flex gap-1 p-1 bg-slate-100 rounded-xl w-fit flex-wrap">
        {(
          [
            ['agents', '员工'],
            ['topology', '通讯拓扑'],
            ['mails', '内部邮件'],
          ] as const
        ).map(([k, label]) => (
          <button
            key={k}
            type="button"
            onClick={() => setTab(k)}
            className={`px-4 py-2 rounded-lg text-sm font-medium transition-colors ${
              tab === k ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-600 hover:text-slate-900'
            }`}
          >
            {label}
          </button>
        ))}
      </div>

      {err && (
        <div className="rounded-lg border border-rose-200 bg-rose-50 text-rose-800 text-sm px-4 py-3">{err}</div>
      )}

      {loading ? (
        <p className="text-slate-500">加载中…</p>
      ) : (
        <>
          {tab === 'agents' && (
            <div className="rounded-2xl border border-slate-200 bg-white p-4 sm:p-6 shadow-sm space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h2 className="font-medium text-slate-800">员工列表</h2>
                <span className="text-xs text-slate-400 tabular-nums">
                  表格 {agentRows.length}/{maxAgentsCap} 行 · 将保存 {validAgentRowCount}/{maxAgentsCap} 名
                </span>
              </div>
              <p className="text-xs text-slate-500">
                <code className="bg-slate-100 px-1 rounded">agent_slug</code> 须与容器内{' '}
                <code className="bg-slate-100 px-1 rounded">agents.list[].id</code> 一致。单个实例最多 {maxAgentsCap}{' '}
                名员工。
              </p>
              <div className="space-y-2">
                {agentRows.map((row, i) => (
                  <div key={i} className="flex flex-wrap gap-2 items-center">
                    <input
                      className="flex-1 min-w-[120px] border border-slate-200 rounded-lg px-3 py-2 text-sm"
                      placeholder="agent_slug（如 main）"
                      maxLength={maxSlugRunes}
                      value={row.agent_slug}
                      onChange={(e) => {
                        const next = [...agentRows]
                        next[i] = { ...next[i], agent_slug: e.target.value }
                        setAgentRows(next)
                      }}
                    />
                    <input
                      className="flex-1 min-w-[120px] border border-slate-200 rounded-lg px-3 py-2 text-sm"
                      placeholder="展示名（账号内唯一）"
                      maxLength={maxDisplayRunes}
                      value={row.display_name}
                      onChange={(e) => {
                        const next = [...agentRows]
                        next[i] = { ...next[i], display_name: e.target.value }
                        setAgentRows(next)
                      }}
                    />
                    <button
                      type="button"
                      className="text-sm text-rose-600 hover:underline"
                      onClick={() => setAgentRows(agentRows.filter((_, j) => j !== i))}
                    >
                      删除
                    </button>
                  </div>
                ))}
              </div>
              <button
                type="button"
                disabled={!canAddAgentRow}
                className="text-sm text-indigo-600 hover:underline disabled:opacity-40 disabled:cursor-not-allowed disabled:no-underline"
                onClick={() => setAgentRows([...agentRows, { agent_slug: '', display_name: '' }])}
              >
                + 添加一行
              </button>
              <div>
                <button
                  type="button"
                  disabled={saving}
                  onClick={saveAgents}
                  className="mt-2 px-4 py-2 rounded-xl bg-indigo-600 text-white text-sm font-medium hover:bg-indigo-700 disabled:opacity-50"
                >
                  {saving ? '保存中…' : '保存员工'}
                </button>
              </div>

              <div className="border-t border-slate-100 pt-4 mt-4">
                <h3 className="text-sm font-medium text-slate-700 mb-2">测试指人解析</h3>
                <div className="flex flex-wrap gap-2 items-center">
                  <input
                    className="border border-slate-200 rounded-lg px-3 py-2 text-sm flex-1 min-w-[160px]"
                    placeholder="输入展示名"
                    maxLength={maxDisplayRunes}
                    value={resolveName}
                    onChange={(e) => setResolveName(e.target.value)}
                  />
                  <button
                    type="button"
                    onClick={tryResolve}
                    className="px-3 py-2 rounded-lg bg-slate-100 text-slate-800 text-sm hover:bg-slate-200"
                  >
                    解析
                  </button>
                </div>
                {resolveOut && <p className="mt-2 text-sm text-slate-600">{resolveOut}</p>}
              </div>
            </div>
          )}

          {tab === 'topology' && (
            <div className="rounded-2xl border border-slate-200 bg-white p-4 sm:p-6 shadow-sm space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h2 className="font-medium text-slate-800">无向连线</h2>
                <span className="text-xs text-slate-400 tabular-nums">
                  边 {edges.length}/{maxEdgesCap} · 拓扑版本 {topoVersion}
                </span>
              </div>
              <p className="text-xs text-slate-500">
                两端即互为邻居，内部邮件仅允许发给邻居。请先保存员工列表再添加边。无向边合计最多 {maxEdgesCap}{' '}
                条。
              </p>
              <div className="rounded-lg border border-violet-100 bg-violet-50/80 px-3 py-2 text-xs text-violet-900 flex flex-wrap items-center justify-between gap-2">
                <span>更习惯在首页用画布点选连线？</span>
                <button
                  type="button"
                  onClick={() => navigate('/', { state: { orchestrateInstanceId: instanceId } })}
                  className="shrink-0 px-2 py-1 rounded-md bg-white border border-violet-200 text-violet-800 hover:bg-violet-100"
                >
                  打开首页编排画布
                </button>
              </div>
              <div className="flex flex-wrap gap-2 items-end">
                <div>
                  <label className="block text-xs text-slate-500 mb-1">员工 A</label>
                  <select
                    className="border border-slate-200 rounded-lg px-3 py-2 text-sm min-w-[140px]"
                    value={edgeA}
                    onChange={(e) => setEdgeA(e.target.value)}
                  >
                    <option value="">选择</option>
                    {slugOptions.map((s) => (
                      <option key={s} value={s}>
                        {s}
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="block text-xs text-slate-500 mb-1">员工 B</label>
                  <select
                    className="border border-slate-200 rounded-lg px-3 py-2 text-sm min-w-[140px]"
                    value={edgeB}
                    onChange={(e) => setEdgeB(e.target.value)}
                  >
                    <option value="">选择</option>
                    {slugOptions.map((s) => (
                      <option key={s} value={s}>
                        {s}
                      </option>
                    ))}
                  </select>
                </div>
                <button
                  type="button"
                  disabled={atEdgeLimit}
                  onClick={addEdge}
                  className="px-3 py-2 rounded-lg bg-slate-100 text-sm text-slate-800 hover:bg-slate-200 disabled:opacity-40 disabled:cursor-not-allowed"
                >
                  添加连线
                </button>
              </div>
              <ul className="divide-y divide-slate-100 border border-slate-100 rounded-lg">
                {edges.length === 0 && <li className="px-3 py-4 text-sm text-slate-400">暂无连线</li>}
                {edges.map(([a, b], i) => (
                  <li key={`${a}-${b}-${i}`} className="flex items-center justify-between px-3 py-2 text-sm">
                    <span>
                      <code className="bg-slate-50 px-1 rounded">{a}</code> ↔{' '}
                      <code className="bg-slate-50 px-1 rounded">{b}</code>
                    </span>
                    <button type="button" className="text-rose-600 text-xs hover:underline" onClick={() => removeEdge(i)}>
                      移除
                    </button>
                  </li>
                ))}
              </ul>
              <button
                type="button"
                disabled={saving}
                onClick={saveTopology}
                className="px-4 py-2 rounded-xl bg-indigo-600 text-white text-sm font-medium hover:bg-indigo-700 disabled:opacity-50"
              >
                {saving ? '保存中…' : '保存拓扑'}
              </button>
            </div>
          )}

          {tab === 'mails' && (
            <div className="rounded-2xl border border-slate-200 bg-white p-4 sm:p-6 shadow-sm space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h2 className="font-medium text-slate-800">内部邮件记录</h2>
                <span className="text-xs text-slate-400 tabular-nums">
                  每页 {mailPageSize} 条 · offset 上限 {maxMailListOffsetCap.toLocaleString()}
                </span>
              </div>
              {mailListErr && (
                <div className="rounded-lg border border-rose-200 bg-rose-50 text-rose-800 text-sm px-4 py-3">
                  {mailListErr}
                </div>
              )}
              <div className="flex flex-wrap gap-2 items-center">
                <input
                  className="border border-slate-200 rounded-lg px-3 py-2 text-sm flex-1 min-w-[200px]"
                  placeholder="按 thread_id 筛选（可选，输入后约 0.4s 查询）"
                  maxLength={maxThreadFilterRunes}
                  value={mailThreadFilterInput}
                  onChange={(e) => setMailThreadFilterInput(e.target.value)}
                />
                <button
                  type="button"
                  disabled={mailListLoading}
                  onClick={() => void loadMails({ thread: mailThreadFilterInput })}
                  className="px-3 py-2 rounded-lg bg-slate-100 text-sm hover:bg-slate-200 disabled:opacity-50"
                >
                  {mailListLoading ? '加载中…' : '刷新'}
                </button>
              </div>
              <p className="text-xs text-slate-500">
                {mailListLoading && mails.length > 0 && (
                  <span className="text-indigo-600 mr-2">更新中…</span>
                )}
                {mailActiveThread
                  ? `已筛选单线程，按 id 正序；${mailTotal != null ? `已加载 ${mails.length} / 共 ${mailTotal} 条` : `已加载 ${mails.length} 条`}。`
                  : `按会话分组（最近活跃在上），每组内按 id 正序；${mailTotal != null ? `已加载 ${mails.length} / 共 ${mailTotal} 条` : `已加载 ${mails.length} 条`}。`}
              </p>
              <div className="space-y-2">
                {mailThreads.map(({ threadId, mails: threadMails, lastAt }) => (
                  <details
                    key={threadId}
                    className="border border-slate-200 rounded-xl overflow-hidden bg-slate-50/50"
                  >
                    <summary className="px-3 py-2.5 cursor-pointer list-none flex flex-wrap items-center gap-2 text-sm bg-white hover:bg-slate-50 [&::-webkit-details-marker]:hidden">
                      <span className="text-slate-400 select-none">▸</span>
                      <code className="text-xs bg-slate-100 px-1.5 py-0.5 rounded max-w-[min(100%,280px)] truncate" title={threadId}>
                        {threadId}
                      </code>
                      {threadId !== '—' && (
                        <button
                          type="button"
                          className="text-xs text-indigo-600 hover:underline shrink-0"
                          onClick={(e) => {
                            e.preventDefault()
                            e.stopPropagation()
                            copyThreadId(threadId)
                          }}
                        >
                          {copiedThreadTip === threadId ? '已复制' : '复制 thread'}
                        </button>
                      )}
                      <span className="text-slate-500">{threadMails.length} 封</span>
                      {lastAt && <span className="text-xs text-slate-400 ml-auto">{lastAt}</span>}
                    </summary>
                    <div className="border-t border-slate-200 bg-white px-3 py-2 space-y-3">
                      {threadMails.map((m) => (
                        <div key={m.id} className="text-sm border-b border-slate-100 last:border-0 pb-3 last:pb-0">
                          <div className="flex flex-wrap gap-x-2 gap-y-0.5 text-xs text-slate-500">
                            <span>#{m.id}</span>
                            <span>{m.created_at}</span>
                            <span>
                              {m.from_slug} → {m.to_slug}
                            </span>
                            {m.in_reply_to != null && <span>↩ {m.in_reply_to}</span>}
                          </div>
                          <div className="font-medium text-slate-800 mt-1">{m.subject || '—'}</div>
                          <div className="text-xs text-slate-600 mt-1 whitespace-pre-wrap break-words">{m.body}</div>
                        </div>
                      ))}
                    </div>
                  </details>
                ))}
                {mails.length === 0 &&
                  (mailListLoading ? (
                    <p className="text-slate-500 text-sm py-8 text-center">加载邮件…</p>
                  ) : (
                    <p className="text-slate-400 text-sm py-6 text-center">暂无邮件</p>
                  ))}
              </div>
              {mailHasMore && mails.length > 0 && (
                <button
                  type="button"
                  disabled={mailLoadingMore}
                  onClick={() => loadMoreMails()}
                  className="w-full py-2 rounded-lg border border-slate-200 text-sm text-slate-700 hover:bg-slate-50 disabled:opacity-50"
                >
                  {mailLoadingMore ? '加载中…' : '加载更多'}
                </button>
              )}
            </div>
          )}
        </>
      )}
    </div>
  )
}

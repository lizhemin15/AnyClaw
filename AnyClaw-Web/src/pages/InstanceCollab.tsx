import { useState, useEffect, useCallback, useMemo } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { SafeLink } from '../components/SafeLink'
import CollabTopologyPanel from '../components/CollabTopologyPanel'
import {
  broadcastCollabEvent,
  getInstance,
  getCollabAgents,
  putCollabAgents,
  postCollabResolve,
  type CollabApiError,
  ANYCLAW_COLLAB_BROADCAST,
  type CollabAgent,
  type CollabLimits,
} from '../api'

export default function InstanceCollab() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const instanceId = parseInt(id || '', 10)

  const [instanceName, setInstanceName] = useState('')
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const [agentRows, setAgentRows] = useState<{ agent_slug: string; display_name: string }[]>([])
  const [resolveName, setResolveName] = useState('')
  const [resolveOut, setResolveOut] = useState<string | null>(null)
  const [collabLimits, setCollabLimits] = useState<CollabLimits | null>(null)
  /** 员工名单保存后递增，刷新下方拓扑画布节点 */
  const [rosterRevision, setRosterRevision] = useState(0)

  const maxAgentsCap = collabLimits?.max_agents ?? 64
  const maxSlugRunes = collabLimits?.max_agent_slug_runes ?? 128
  const maxDisplayRunes = collabLimits?.max_agent_display_name_runes ?? 255
  const canAddAgentRow = agentRows.length < maxAgentsCap

  const validAgentRowCount = useMemo(
    () => agentRows.filter((r) => r.agent_slug.trim() !== '' && r.display_name.trim() !== '').length,
    [agentRows]
  )

  const loadAgents = useCallback(async () => {
    const { agents, limits } = await getCollabAgents(instanceId)
    setCollabLimits(limits ?? null)
    setAgentRows(
      agents.length
        ? agents.map((a: CollabAgent) => ({ agent_slug: a.agent_slug, display_name: a.display_name }))
        : [{ agent_slug: 'main', display_name: '主理' }]
    )
  }, [instanceId])

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
      } catch (e) {
        if (!cancelled) setErr(e instanceof Error ? e.message : String(e))
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [instanceId, loadAgents])

  useEffect(() => {
    if (isNaN(instanceId) || typeof BroadcastChannel === 'undefined') return
    let bc: BroadcastChannel | null = null
    try {
      bc = new BroadcastChannel(ANYCLAW_COLLAB_BROADCAST)
      bc.onmessage = (ev: MessageEvent) => {
        const d = ev.data as { kind?: string; instanceId?: number }
        if (d.instanceId !== instanceId) return
        if (d.kind === 'topology') {
          void loadAgents()
        }
      }
    } catch {
      /* noop */
    }
    return () => {
      bc?.close()
    }
  }, [instanceId, loadAgents])

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
      setRosterRevision((n) => n + 1)
    } catch (e) {
      const lim = (e as CollabApiError).collabLimits
      if (lim) setCollabLimits(lim)
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
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
    <div className="max-w-4xl mx-auto space-y-6">
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
          首页编排与邮件
        </button>
        <span className="text-slate-300">|</span>
        <h1 className="text-lg font-semibold text-slate-800">协作展示名 · {instanceName || `#${instanceId}`}</h1>
      </div>
      <p className="text-sm text-slate-500">
        协作员工名单与容器 <code className="bg-slate-100 px-1 rounded text-xs">agents.list</code> 同步后会在 API 侧持久化，打开编排/本页时会自动补全节点；下方可编辑展示名（账号内唯一）并编排通讯拓扑。
        内部邮件请在首页「邮箱」或编排弹窗「邮件」中查看。
      </p>

      {err && (
        <div className="rounded-lg border border-rose-200 bg-rose-50 text-rose-800 text-sm px-4 py-3">{err}</div>
      )}

      {loading ? (
        <p className="text-slate-500">加载中…</p>
      ) : (
        <div className="rounded-2xl border border-slate-200 bg-white p-4 sm:p-6 shadow-sm space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h2 className="font-medium text-slate-800">员工展示名</h2>
            <span className="text-xs text-slate-400 tabular-nums">
              表格 {agentRows.length}/{maxAgentsCap} 行 · 将保存 {validAgentRowCount}/{maxAgentsCap} 名
            </span>
          </div>
          <p className="text-xs text-slate-500">
            <code className="bg-slate-100 px-1 rounded">agent_slug</code> 须与容器内{' '}
            <code className="bg-slate-100 px-1 rounded">agents.list[].id</code> 一致。单个实例最多 {maxAgentsCap} 名员工。
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

      {!loading && (
        <CollabTopologyPanel instanceId={instanceId} rosterRevision={rosterRevision} />
      )}
    </div>
  )
}

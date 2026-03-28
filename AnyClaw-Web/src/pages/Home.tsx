import { useState, useEffect, useMemo, useRef } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import {
  getInstances,
  createInstance,
  deleteInstance,
  subscribeInstance,
  updateInstanceName,
  getAuthConfig,
  getEnergyConfig,
  getCollabAgents,
  type CollabPeerInstance,
  type Instance,
  type User,
} from '../api'
import { mergeInstancesWithPeers } from '../components/collabTopologyUtils'
import SearchInput from '../components/SearchInput'
import Pagination from '../components/Pagination'
import HomeCollabOrchestrateModal from '../components/HomeCollabOrchestrateModal'
import CompanyMailsModal from '../components/CompanyMailsModal'
import InstanceMailPanel from '../components/InstanceMailPanel'

const PAGE_SIZE = 8

export default function Home({ user, onRefresh, showGuide = false, onDismissGuide }: { user: User | null; onRefresh?: () => void; showGuide?: boolean; onDismissGuide?: () => void }) {
  const [instances, setInstances] = useState<Instance[]>([])
  const [guideStep, setGuideStep] = useState(1)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [creating, setCreating] = useState(false)
  const [newName, setNewName] = useState('')
  const [deleting, setDeleting] = useState<number | null>(null)
  const [subscribing, setSubscribing] = useState<number | null>(null)
  const [adoptCost, setAdoptCost] = useState(100)
  const [monthlyCost, setMonthlyCost] = useState(0)
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [editDraft, setEditDraft] = useState('')
  const [renamingId, setRenamingId] = useState<number | null>(null)
  const nameInputRef = useRef<HTMLInputElement>(null)
  const skipNameBlurCommit = useRef(false)
  const editingIdRef = useRef<number | null>(null)
  editingIdRef.current = editingId

  const [orchMode, setOrchMode] = useState(false)
  const [orchInlineInst, setOrchInlineInst] = useState<Instance | null>(null)
  const [instanceListRevision, setInstanceListRevision] = useState(0)
  const [companyMailOpen, setCompanyMailOpen] = useState(false)
  const [instanceMailOpen, setInstanceMailOpen] = useState(false)

  const navigate = useNavigate()
  const location = useLocation()

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return instances
    return instances.filter((i) => i.name.toLowerCase().includes(q))
  }, [instances, search])

  const paginated = useMemo(() => {
    const start = (page - 1) * PAGE_SIZE
    return filtered.slice(start, start + PAGE_SIZE)
  }, [filtered, page])

  useEffect(() => {
    const st = location.state as {
      orchestrateInstanceId?: number
      orchestrateCollabTab?: 'topo' | 'mails'
    } | null
    const sid = st?.orchestrateInstanceId
    if (sid == null || typeof sid !== 'number') return
    if (loading) return
    const match = instances.find((i) => i.id === sid)
    if (match) {
      setOrchMode(true)
      setOrchInlineInst(match)
      if (st?.orchestrateCollabTab === 'mails') setCompanyMailOpen(true)
    }
    navigate(`${location.pathname}${location.search}`, { replace: true, state: {} })
  }, [loading, instances, location.state, location.pathname, location.search, navigate])

  useEffect(() => {
    if (!orchMode) setOrchInlineInst(null)
  }, [orchMode])

  useEffect(() => {
    if (!orchMode || instances.length === 0) return
    setOrchInlineInst((prev) => {
      if (prev && instances.some((i) => i.id === prev.id)) return prev
      return filtered[0] ?? instances[0]
    })
  }, [orchMode, instances, filtered])

  /** 编排模式开启时：对每个实例请求协作名单以触发 API 同步（工作区 agents.list、协作表补全） */
  useEffect(() => {
    if (!orchMode || instances.length === 0) return
    void Promise.allSettled(instances.map((i) => getCollabAgents(i.id)))
  }, [orchMode, instances])

  useEffect(() => {
    setPage(1)
  }, [search])

  useEffect(() => {
    if (editingId != null) {
      nameInputRef.current?.focus()
      nameInputRef.current?.select()
    }
  }, [editingId])

  const formatExpires = (s: string) => {
    if (!s || s.length < 10) return ''
    const [, m, d] = s.slice(0, 10).split('-')
    return `${parseInt(m, 10)}月${parseInt(d, 10)}日`
  }

  useEffect(() => {
    const load = async () => {
      try {
        const c = await getAuthConfig()
        const cost = c.adopt_cost ?? 0
        if (cost > 0) setAdoptCost(cost)
      } catch {}
      try {
        const e = await getEnergyConfig()
        if ((e.adopt_cost ?? 0) > 0) setAdoptCost(e.adopt_cost!)
        if ((e.monthly_subscription_cost ?? 0) > 0) setMonthlyCost(e.monthly_subscription_cost!)
      } catch {}
    }
    load()
  }, [])

  const loadInstances = () => {
    setLoading(true)
    ;(async () => {
      try {
        const list = await getInstances()
        let merged = list
        if (list.length > 0) {
          const peerBatches = await Promise.all(
            list.map((i) =>
              getCollabAgents(i.id)
                .then((r) => r.peer_instances ?? [])
                .catch((): CollabPeerInstance[] => [])
            )
          )
          merged = mergeInstancesWithPeers(list, peerBatches.flat())
        }
        setInstances(merged)
        setInstanceListRevision((n) => n + 1)
      } catch (err) {
        setError(err instanceof Error ? err.message : '加载失败')
      } finally {
        setLoading(false)
      }
    })()
  }

  useEffect(() => {
    loadInstances()
    // 从 Chat 等页面返回时刷新，以显示 LLM/WS 纠正后的状态
    const onFocus = () => loadInstances()
    window.addEventListener('focus', onFocus)
    return () => window.removeEventListener('focus', onFocus)
  }, [])

  const handleAdopt = async (e: React.FormEvent) => {
    e.preventDefault()
    if (creating) return
    const name = newName.trim() || '小爪'
    if (!user) return
    if (user.energy < adoptCost) {
      setError(`金币不足，招聘需要 ${adoptCost} 金币`)
      return
    }
    setCreating(true)
    setError('')
    try {
      const inst = await createInstance(name)
      setInstances((prev) => [inst, ...prev])
      setNewName('')
      onRefresh?.()
      navigate(`/instances/${inst.id}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : '招聘失败')
    } finally {
      setCreating(false)
    }
  }

  const handleAbandon = async (e: React.MouseEvent, inst: Instance) => {
    e.stopPropagation()
    if (!confirm(`确定解雇「${inst.name}」？解雇后无法恢复，系统将删除该员工的所有聊天记录。`)) return
    setDeleting(inst.id)
    setError('')
    try {
      await deleteInstance(inst.id)
      setInstances((prev) => prev.filter((i) => i.id !== inst.id))
    } catch (err) {
      setError(err instanceof Error ? err.message : '解雇失败')
    } finally {
      setDeleting(null)
    }
  }

  const cancelNameEdit = () => {
    skipNameBlurCommit.current = true
    setEditingId(null)
    setEditDraft('')
    queueMicrotask(() => {
      skipNameBlurCommit.current = false
    })
  }

  const commitNameEdit = async (inst: Instance, nameOverride?: string) => {
    const next = (nameOverride ?? editDraft).trim()
    if (!next) {
      setError('名称不能为空')
      nameInputRef.current?.focus()
      return
    }
    if (next === inst.name) {
      if (editingIdRef.current === inst.id) cancelNameEdit()
      return
    }
    setRenamingId(inst.id)
    setError('')
    try {
      const updated = await updateInstanceName(inst.id, next)
      setInstances((prev) => prev.map((i) => (i.id === inst.id ? updated : i)))
      if (editingIdRef.current === inst.id) cancelNameEdit()
    } catch (err) {
      setError(err instanceof Error ? err.message : '修改失败')
    } finally {
      setRenamingId(null)
    }
  }

  const handleNameBlur = (inst: Instance, valueFromInput: string) => {
    window.setTimeout(() => {
      if (skipNameBlurCommit.current) return
      void commitNameEdit(inst, valueFromInput)
    }, 0)
  }

  const handleSubscribe = async (e: React.MouseEvent, inst: Instance) => {
    e.stopPropagation()
    if (subscribing || !user) return
    if (user.energy < monthlyCost) {
      setError(`金币不足，包月需要 ${monthlyCost} 金币`)
      return
    }
    if (!confirm(`确定要为「${inst.name}」包月吗？将消耗 ${monthlyCost} 金币，30 天内对话不再消耗金币。`)) return
    setSubscribing(inst.id)
    setError('')
    try {
      const updated = await subscribeInstance(inst.id)
      setInstances((prev) => prev.map((i) => (i.id === inst.id ? updated : i)))
      onRefresh?.()
    } catch (err) {
      setError(err instanceof Error ? err.message : '包月失败')
    } finally {
      setSubscribing(null)
    }
  }

  return (
    <div className="max-w-4xl mx-auto relative">
      {/* 新手引导 */}
      {showGuide && (
        <div className="fixed inset-0 z-50 bg-black/60 flex items-center justify-center p-4">
          <div className="bg-slate-800 text-white rounded-xl p-6 max-w-sm shadow-2xl">
            {guideStep === 1 && (
              <>
                <p className="text-lg font-bold">👋 欢迎！</p>
                <p className="text-slate-300 mt-2 text-sm">招聘你的第一名 AI 员工，开启一人公司效率底座，输入名字后点击招聘</p>
                <button type="button" onClick={() => setGuideStep(2)} className="mt-4 w-full py-2 bg-white text-slate-800 rounded-lg font-medium">下一步</button>
              </>
            )}
            {guideStep === 2 && (
              <>
                <p className="text-lg font-bold">📋 我的公司</p>
                <p className="text-slate-300 mt-2 text-sm">招聘的员工会显示在这里，点击可进入对话</p>
                <button type="button" onClick={() => setGuideStep(3)} className="mt-4 w-full py-2 bg-white text-slate-800 rounded-lg font-medium">下一步</button>
              </>
            )}
            {guideStep === 3 && (
              <>
                <p className="text-lg font-bold">💬 开始对话</p>
                <p className="text-slate-300 mt-2 text-sm">点击员工卡片即可打开对话，用效率工具完成复杂任务</p>
                <button type="button" onClick={() => { onDismissGuide?.(); setGuideStep(1); }} className="mt-4 w-full py-2 bg-emerald-500 text-white rounded-lg font-medium">知道了</button>
              </>
            )}
          </div>
        </div>
      )}
      {/* 金币与充值 */}
      <div className="mb-4 p-3 sm:p-4 bg-white rounded-xl border border-slate-200 flex flex-wrap items-center justify-between gap-2 sm:gap-3">
        <div className="flex items-center gap-2 sm:gap-3">
          <span className="text-sm text-slate-600 hidden sm:inline">我的金币</span>
          <span className="text-xl font-bold text-slate-800">🪙 {user?.energy ?? 0}</span>
        </div>
        <div className="flex gap-1.5 sm:gap-2">
          <Link
            to="/usage"
            className="px-3 py-1.5 sm:px-4 sm:py-2 text-sm border border-slate-300 rounded-lg active:bg-slate-50"
          >
            消耗
          </Link>
          <Link
            to="/recharge"
            className="px-3 py-1.5 sm:px-4 sm:py-2 text-sm font-medium bg-amber-500 text-amber-950 rounded-lg hover:bg-amber-400 active:bg-amber-600"
          >
            充值
          </Link>
        </div>
      </div>

      {/* 招聘新员工 */}
      <div className="mb-6">
        <div className="flex items-center gap-3 mb-2 sm:mb-3">
          <img src="/10002.svg" alt="" className="w-12 h-12 sm:w-14 sm:h-14 flex-shrink-0" aria-hidden />
          <div>
            <h2 className="text-base sm:text-lg font-semibold text-slate-800">招聘 AI 员工</h2>
            <p className="text-sm text-slate-500">OpenClaw 是效率工具、一人公司的底座。每位员工擅长复杂任务、拥有超长记忆，回答会稍慢一些～</p>
          </div>
        </div>
        <p className="mt-2 mb-3 text-sm text-slate-600 leading-relaxed rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5">
          OpenClaw 是硅基生物，拥有心跳、思考、主动发消息等特性，这一切都需要消耗 token，所以建议包月。
        </p>
        <form onSubmit={handleAdopt} className="flex flex-col sm:flex-row gap-3">
          <input
            type="text"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            placeholder="填写员工姓名"
            className="flex-1 px-4 py-3 border border-slate-300 rounded-xl"
          />
          <button
            type="submit"
            disabled={creating || (user?.energy ?? 0) < adoptCost}
            className="px-6 py-3 bg-slate-800 text-white rounded-xl disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {creating ? '招聘中，请稍候（约 1–2 分钟）...' : `招聘 (${adoptCost} 金币)`}
          </button>
        </form>
        {(user?.energy ?? 0) < adoptCost && (
          <p className="mt-2 text-sm text-amber-600">金币不足，可通过充值获取</p>
        )}
      </div>

      {error && (
        <p className="mb-4 text-sm text-red-600 bg-red-50 p-3 rounded-lg">{error}</p>
      )}

      {/* 公司：搜索、分页 */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-3">
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="text-base sm:text-lg font-semibold text-slate-800">我的公司</h2>
          {instances.length > 0 && (
            <>
              <button
                type="button"
                onClick={() => setOrchMode((v) => !v)}
                className={`px-2.5 py-1 text-xs font-medium rounded-lg border transition-colors ${
                  orchMode
                    ? 'border-violet-500 bg-violet-50 text-violet-800'
                    : 'border-slate-200 text-slate-600 hover:bg-slate-50'
                }`}
                title="开启后在本页编排通讯拓扑；点击卡片切换要编辑的员工（实例）"
              >
                {orchMode ? '编排模式（开）' : '编排模式'}
              </button>
              <button
                type="button"
                onClick={() => setCompanyMailOpen(true)}
                className="px-2.5 py-1 text-xs font-medium rounded-lg border border-slate-200 text-slate-600 hover:bg-slate-50"
                title="查看全部员工的内部邮件往来"
              >
                邮箱
              </button>
              <button
                type="button"
                onClick={() => setInstanceMailOpen(true)}
                className="px-2.5 py-1 text-xs font-medium rounded-lg border border-slate-200 text-slate-600 hover:bg-slate-50"
                title="查看实例之间的跨实例消息往来"
              >
                跨实例信箱
              </button>
            </>
          )}
        </div>
        {instances.length > 0 && (
          <div className="flex flex-col sm:flex-row gap-2 sm:items-center">
            <SearchInput value={search} onChange={setSearch} placeholder="按名称搜索员工" className="sm:w-48" />
          </div>
        )}
      </div>
      {orchMode && instances.length > 0 && (
        <p className="text-xs text-violet-700 bg-violet-50 border border-violet-100 rounded-lg px-3 py-2 mb-3">
          已开启编排模式：拓扑图展示全部员工实例，可拖拽或点击连线；点击卡片切换当前实例；点卡片内「对话 →」进入聊天。
        </p>
      )}
      {orchMode && orchInlineInst && (
        <div className="mb-4">
          <HomeCollabOrchestrateModal
            key={orchInlineInst.id}
            variant="inline"
            open
            instanceId={orchInlineInst.id}
            instanceName={orchInlineInst.name}
            onClose={() => setOrchMode(false)}
            onSaved={loadInstances}
            rosterRevision={instanceListRevision}
          />
        </div>
      )}
      {loading ? (
        <p className="text-slate-500 py-8">加载中...</p>
      ) : instances.length === 0 ? (
        <div className="text-center py-12 bg-slate-50 rounded-xl">
          <img src="/10003.png" alt="" className="w-24 h-24 mx-auto mb-3 object-contain" aria-hidden />
          <p className="text-slate-500 mb-2">暂无员工</p>
          <p className="text-sm text-slate-400">招聘一名 AI 员工，开启你的效率之旅</p>
        </div>
      ) : filtered.length === 0 ? (
        <div className="text-center py-12 bg-slate-50 rounded-xl">
          <p className="text-slate-500">未找到匹配「{search}」的员工</p>
        </div>
      ) : (
        <>
        <div className={`grid gap-4 sm:grid-cols-2 ${orchMode ? 'ring-2 ring-violet-200 rounded-xl p-1 -m-1' : ''}`}>
          {paginated.map((inst) => (
            <div
              key={inst.id}
              onClick={() => {
                if (orchMode) {
                  setOrchInlineInst(inst)
                  return
                }
                navigate(`/instances/${inst.id}`)
              }}
              className={`bg-white border rounded-xl p-4 transition-colors relative ${
                orchMode
                  ? orchInlineInst?.id === inst.id
                    ? 'border-violet-500 ring-2 ring-violet-200 cursor-pointer bg-violet-50/30'
                    : 'border-violet-200 cursor-pointer hover:bg-violet-50/40'
                  : 'border-slate-200 active:bg-slate-50 cursor-pointer'
              }`}
            >
              <div className="flex items-start justify-between gap-2">
                <div className="flex items-center gap-3 min-w-0 flex-1">
                  <div className="relative flex-shrink-0">
                    <img src="/10001.png" alt="" className="w-10 h-10 object-contain" aria-hidden />
                    {inst.unread && (
                      <span className="absolute -top-0.5 -right-0.5 w-2.5 h-2.5 rounded-full bg-red-500 border-2 border-white" title="新消息" aria-label="新消息" />
                    )}
                  </div>
                  {editingId === inst.id ? (
                    <input
                      ref={nameInputRef}
                      type="text"
                      value={editDraft}
                      onChange={(e) => setEditDraft(e.target.value)}
                      onClick={(e) => e.stopPropagation()}
                      onMouseDown={(e) => e.stopPropagation()}
                      onBlur={(e) => handleNameBlur(inst, (e.target as HTMLInputElement).value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          e.preventDefault()
                          void commitNameEdit(inst)
                        } else if (e.key === 'Escape') {
                          e.preventDefault()
                          cancelNameEdit()
                        }
                      }}
                      disabled={renamingId === inst.id}
                      className="font-medium text-slate-800 min-w-0 flex-1 max-w-full px-2 py-0.5 border border-slate-300 rounded-lg text-sm"
                      maxLength={255}
                      aria-label="员工名称"
                    />
                  ) : (
                    <div className="flex flex-col items-start gap-0.5 min-w-0">
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation()
                          setEditingId(inst.id)
                          setEditDraft(inst.name)
                          setError('')
                        }}
                        className="font-medium text-slate-800 truncate text-left min-w-0 hover:text-slate-600 underline-offset-2 hover:underline"
                        title="点击修改名称"
                      >
                        {inst.name}
                      </button>
                      {orchMode && (
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation()
                            navigate(`/instances/${inst.id}`)
                          }}
                          className="text-[11px] text-slate-500 hover:text-indigo-600"
                        >
                          对话 →
                        </button>
                      )}
                    </div>
                  )}
                </div>
                <div className="flex items-center gap-2 flex-shrink-0">
                  {inst.subscribed_until && (
                    <span className="px-2.5 py-1 text-xs rounded-full bg-emerald-100 text-emerald-800" title={`包月有效期内对话不消耗金币，到期 ${formatExpires(inst.subscribed_until)}`}>
                      包月至 {formatExpires(inst.subscribed_until)}
                    </span>
                  )}
                  {monthlyCost > 0 && !inst.subscribed_until && (
                    <button
                      onClick={(e) => handleSubscribe(e, inst)}
                      disabled={!!subscribing || (user?.energy ?? 0) < monthlyCost}
                      className="px-2 py-1 text-xs text-emerald-600 border border-emerald-200 rounded-lg active:bg-emerald-50 disabled:opacity-50"
                      title={`包月 ${monthlyCost} 金币，30 天内对话不消耗金币`}
                    >
                      {subscribing === inst.id ? '包月中...' : `包月(${monthlyCost})`}
                    </button>
                  )}
                  <span
                    className={`px-2.5 py-1 text-xs rounded-full ${
                      inst.status === 'running'
                        ? 'bg-green-100 text-green-800'
                        : inst.status === 'creating'
                        ? 'bg-amber-100 text-amber-800'
                        : inst.status === 'error'
                        ? 'bg-red-100 text-red-800'
                        : 'bg-slate-100 text-slate-700'
                    }`}
                  >
                    {inst.status === 'running' ? '在线' : inst.status === 'creating' ? '创建中' : inst.status === 'error' ? '异常' : inst.status}
                  </span>
                  <button
                    onClick={(e) => handleAbandon(e, inst)}
                    disabled={!!deleting}
                    className="px-2 py-1 text-xs text-red-600 border border-red-200 rounded-lg active:bg-red-50 disabled:opacity-50"
                    title="解雇"
                  >
                    {deleting === inst.id ? '解雇中...' : '解雇'}
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
        {filtered.length > PAGE_SIZE && (
          <Pagination page={page} pageSize={PAGE_SIZE} total={filtered.length} onPageChange={setPage} />
        )}
        </>
      )}

      <CompanyMailsModal open={companyMailOpen} instances={instances} onClose={() => setCompanyMailOpen(false)} />
      <InstanceMailPanel open={instanceMailOpen} instances={instances} onClose={() => setInstanceMailOpen(false)} />
    </div>
  )
}

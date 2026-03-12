import { useState, useEffect } from 'react'
import { getAdminStats, getAdminUsage, checkAndMigrateDb, resetAdminDb, clearToken, type AdminStats, type UsageLogEntryAdmin } from '../api'

const CHART_COLORS = ['#4318FF', '#00B5D8', '#6C63FF', '#05CD99', '#FFB547', '#FF5E7D', '#41B883', '#7983FF']

export default function AdminStats() {
  const [stats, setStats] = useState<AdminStats | null>(null)
  const [usageList, setUsageList] = useState<UsageLogEntryAdmin[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [days, setDays] = useState(7)
  const [resetting, setResetting] = useState(false)
  const [showResetConfirm, setShowResetConfirm] = useState(false)
  const [migrating, setMigrating] = useState(false)

  const load = () => {
    setLoading(true)
    Promise.all([getAdminStats(days), getAdminUsage(100, 0)])
      .then(([s, u]) => {
        setStats(s)
        setUsageList(u.items ?? [])
      })
      .catch((err) => setError(err instanceof Error ? err.message : '加载失败'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    load()
  }, [days])

  const byModel = stats?.by_model ?? []
  const byUser = stats?.by_user ?? []
  const maxModelTokens = Math.max(
    ...(byModel.map((m) => m.prompt_tokens + m.completion_tokens)),
    1
  )

  return (
    <div className="max-w-5xl mx-auto">
      <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-4 mb-6">
        <div>
          <h1 className="text-xl font-semibold text-slate-800">使用统计</h1>
          <p className="text-sm text-slate-500 mt-1">查看 LLM 调用与 Token 消耗情况</p>
        </div>
        <select
          value={days}
          onChange={(e) => setDays(parseInt(e.target.value, 10))}
          className="px-4 py-2 border border-slate-300 rounded-lg text-sm bg-white focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500"
        >
          <option value={1}>最近 1 天</option>
          <option value={7}>最近 7 天</option>
          <option value={30}>最近 30 天</option>
          <option value={90}>最近 90 天</option>
        </select>
      </div>

      {error && (
        <div className="mb-4 p-4 rounded-lg bg-red-50 border border-red-100 text-red-700 text-sm">{error}</div>
      )}

      {/* 数据库维护 - 始终可见，便于结构不一致时修复 */}
      <div className="mb-6 bg-white rounded-lg border border-slate-200 shadow-sm overflow-hidden">
        <div className="px-5 py-4 border-b border-slate-200">
          <h2 className="font-semibold text-slate-800">数据库维护</h2>
          <p className="text-sm text-slate-500 mt-0.5">迭代更新服务后若 MySQL 结构未同步，可一键检查并补齐缺失的表和列</p>
        </div>
        <div className="p-5 flex gap-3">
          <button
            type="button"
            onClick={async () => {
              setMigrating(true)
              setError('')
              try {
                const res = await checkAndMigrateDb()
                alert(res.message || '数据库结构已检查并修复')
                load()
              } catch (e) {
                setError(e instanceof Error ? e.message : '检查修复失败')
              } finally {
                setMigrating(false)
              }
            }}
            disabled={migrating}
            className="px-4 py-2 rounded-lg bg-indigo-600 text-white text-sm font-medium hover:bg-indigo-700 active:bg-indigo-800 disabled:opacity-50"
          >
            {migrating ? '执行中...' : '检查修复数据库'}
          </button>
        </div>
      </div>

      {loading ? (
        <div className="bg-white rounded-lg border border-slate-200 shadow-sm p-8">
          <div className="animate-pulse space-y-4">
            <div className="grid grid-cols-3 gap-4">
              <div className="h-24 bg-slate-100 rounded-lg" />
              <div className="h-24 bg-slate-100 rounded-lg" />
              <div className="h-24 bg-slate-100 rounded-lg" />
            </div>
            <div className="h-48 bg-slate-100 rounded-lg" />
          </div>
        </div>
      ) : stats ? (
        <div className="space-y-6">
          {/* 汇总卡片 - One API Dashboard 风格 */}
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <div className="bg-white rounded-lg border border-slate-200 shadow-sm p-5">
              <p className="text-sm font-medium text-slate-500">调用次数</p>
              <p className="text-2xl font-bold text-indigo-600 mt-1">{stats.total_calls.toLocaleString()}</p>
            </div>
            <div className="bg-white rounded-lg border border-slate-200 shadow-sm p-5">
              <p className="text-sm font-medium text-slate-500">Prompt Tokens</p>
              <p className="text-2xl font-bold text-cyan-600 mt-1">{stats.total_prompt_tokens.toLocaleString()}</p>
            </div>
            <div className="bg-white rounded-lg border border-slate-200 shadow-sm p-5">
              <p className="text-sm font-medium text-slate-500">Completion Tokens</p>
              <p className="text-2xl font-bold text-violet-600 mt-1">{stats.total_completion_tokens.toLocaleString()}</p>
            </div>
          </div>

          {/* 按模型 - 带简易条形图 */}
          {byModel.length > 0 && (
            <div className="bg-white rounded-lg border border-slate-200 shadow-sm overflow-hidden">
              <div className="px-5 py-4 border-b border-slate-200">
                <h2 className="font-semibold text-slate-800">模型使用分布</h2>
                <p className="text-sm text-slate-500 mt-0.5">各模型 Token 消耗对比</p>
              </div>
              <div className="p-5">
                <div className="space-y-4">
                  {byModel.map((m, i) => {
                    const tokens = m.prompt_tokens + m.completion_tokens
                    const pct = (tokens / maxModelTokens) * 100
                    return (
                      <div key={m.model}>
                        <div className="flex justify-between text-sm mb-1">
                          <span className="font-mono text-slate-700 truncate max-w-[200px]">{m.model}</span>
                          <span className="text-slate-500">
                            {tokens.toLocaleString()} tokens · {m.calls} 次
                          </span>
                        </div>
                        <div className="h-2 bg-slate-100 rounded-full overflow-hidden">
                          <div
                            className="h-full rounded-full transition-all"
                            style={{
                              width: `${pct}%`,
                              backgroundColor: CHART_COLORS[i % CHART_COLORS.length],
                            }}
                          />
                        </div>
                      </div>
                    )
                  })}
                </div>
              </div>
            </div>
          )}

          {/* 按模型 - 表格 */}
          {byModel.length > 0 && (
            <div className="bg-white rounded-lg border border-slate-200 shadow-sm overflow-hidden">
              <div className="px-5 py-4 border-b border-slate-200">
                <h2 className="font-semibold text-slate-800">模型明细</h2>
              </div>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead>
                    <tr className="bg-slate-50 border-b border-slate-200">
                      <th className="text-left py-3 px-5 text-sm font-medium text-slate-600">模型</th>
                      <th className="text-right py-3 px-5 text-sm font-medium text-slate-600">调用次数</th>
                      <th className="text-right py-3 px-5 text-sm font-medium text-slate-600">Prompt</th>
                      <th className="text-right py-3 px-5 text-sm font-medium text-slate-600">Completion</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100">
                    {byModel.map((m) => (
                      <tr key={m.model} className="hover:bg-slate-50/50">
                        <td className="py-3 px-5 font-mono text-sm text-slate-800">{m.model}</td>
                        <td className="py-3 px-5 text-right text-sm text-slate-600">{m.calls.toLocaleString()}</td>
                        <td className="py-3 px-5 text-right text-sm text-slate-600">{m.prompt_tokens.toLocaleString()}</td>
                        <td className="py-3 px-5 text-right text-sm text-slate-600">{m.completion_tokens.toLocaleString()}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {/* 消耗明细（管理员：用户、宠物、模型、时间、金额） */}
          {usageList.length > 0 && (
            <div className="bg-white rounded-lg border border-slate-200 shadow-sm overflow-hidden">
              <div className="px-5 py-4 border-b border-slate-200">
                <h2 className="font-semibold text-slate-800">消耗明细</h2>
                <p className="text-sm text-slate-500 mt-0.5">用户、宠物、模型、时间、金额</p>
              </div>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead>
                    <tr className="bg-slate-50 border-b border-slate-200">
                      <th className="text-left py-3 px-5 text-sm font-medium text-slate-600">用户</th>
                      <th className="text-left py-3 px-5 text-sm font-medium text-slate-600">宠物</th>
                      <th className="text-left py-3 px-5 text-sm font-medium text-slate-600">模型</th>
                      <th className="text-left py-3 px-5 text-sm font-medium text-slate-600">时间</th>
                      <th className="text-right py-3 px-5 text-sm font-medium text-slate-600">消耗</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100">
                    {usageList.map((u) => (
                      <tr key={u.id} className="hover:bg-slate-50/50">
                        <td className="py-3 px-5 text-sm text-slate-800">{u.user_email || '—'}</td>
                        <td className="py-3 px-5 text-sm text-slate-800">{u.instance_name || `#${u.instance_id}`}</td>
                        <td className="py-3 px-5 text-sm font-mono text-slate-600 truncate max-w-[200px]">{u.model}</td>
                        <td className="py-3 px-5 text-sm text-slate-600">{u.created_at}</td>
                        <td className="py-3 px-5 text-right text-sm font-medium text-amber-600">-{u.coins_cost}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {/* 按用户 */}
          {byUser.length > 0 && (
            <div className="bg-white rounded-lg border border-slate-200 shadow-sm overflow-hidden">
              <div className="px-5 py-4 border-b border-slate-200">
                <h2 className="font-semibold text-slate-800">用户使用明细</h2>
              </div>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead>
                    <tr className="bg-slate-50 border-b border-slate-200">
                      <th className="text-left py-3 px-5 text-sm font-medium text-slate-600">用户</th>
                      <th className="text-right py-3 px-5 text-sm font-medium text-slate-600">调用次数</th>
                      <th className="text-right py-3 px-5 text-sm font-medium text-slate-600">Prompt</th>
                      <th className="text-right py-3 px-5 text-sm font-medium text-slate-600">Completion</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100">
                    {byUser.map((u) => (
                      <tr key={u.user_id} className="hover:bg-slate-50/50">
                        <td className="py-3 px-5 text-sm text-slate-800">{u.email || u.user_id}</td>
                        <td className="py-3 px-5 text-right text-sm text-slate-600">{u.calls.toLocaleString()}</td>
                        <td className="py-3 px-5 text-right text-sm text-slate-600">{u.prompt_tokens.toLocaleString()}</td>
                        <td className="py-3 px-5 text-right text-sm text-slate-600">{u.completion_tokens.toLocaleString()}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {stats.total_calls === 0 && (
            <div className="bg-white rounded-lg border border-slate-200 shadow-sm p-12 text-center">
              <p className="text-slate-500">暂无使用数据</p>
              <p className="text-sm text-slate-400 mt-1">宠物实例产生对话后将在此展示</p>
            </div>
          )}

          {/* 危险操作：一键重置数据库 */}
          <div className="bg-white rounded-lg border border-red-200 shadow-sm overflow-hidden">
            <div className="px-5 py-4 border-b border-red-100 bg-red-50/50">
              <h2 className="font-semibold text-red-800">危险操作</h2>
              <p className="text-sm text-red-600 mt-0.5">重置将清空所有用户、实例、订单等数据，不可恢复</p>
            </div>
            <div className="p-5">
              <button
                type="button"
                onClick={() => setShowResetConfirm(true)}
                disabled={resetting}
                className="px-4 py-2 rounded-lg bg-red-600 text-white text-sm font-medium hover:bg-red-700 active:bg-red-800 disabled:opacity-50"
              >
                {resetting ? '重置中...' : '一键重置数据库'}
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {showResetConfirm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="bg-white rounded-xl shadow-xl max-w-md w-full p-6">
            <h3 className="text-lg font-semibold text-slate-800">确认重置数据库？</h3>
            <p className="mt-2 text-sm text-slate-600">
              将清空所有用户、宠物实例、订单、消息等数据。重置后需前往设置页重新创建管理员。
            </p>
            <div className="mt-6 flex gap-3 justify-end">
              <button
                type="button"
                onClick={() => setShowResetConfirm(false)}
                className="px-4 py-2 rounded-lg border border-slate-300 text-slate-700 text-sm hover:bg-slate-50"
              >
                取消
              </button>
              <button
                type="button"
                onClick={async () => {
                  setResetting(true)
                  try {
                    await resetAdminDb()
                    clearToken()
                    window.location.href = '/setup'
                  } catch (e) {
                    setError(e instanceof Error ? e.message : '重置失败')
                    setShowResetConfirm(false)
                  } finally {
                    setResetting(false)
                  }
                }}
                disabled={resetting}
                className="px-4 py-2 rounded-lg bg-red-600 text-white text-sm font-medium hover:bg-red-700 disabled:opacity-50"
              >
                {resetting ? '重置中...' : '确认重置'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

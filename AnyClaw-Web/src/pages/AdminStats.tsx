import { useState, useEffect } from 'react'
import { getAdminStats, type AdminStats } from '../api'

export default function AdminStats() {
  const [stats, setStats] = useState<AdminStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [days, setDays] = useState(7)

  const load = () => {
    setLoading(true)
    getAdminStats(days)
      .then(setStats)
      .catch((err) => setError(err instanceof Error ? err.message : '加载失败'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    load()
  }, [days])

  return (
    <div className="max-w-4xl mx-auto">
      <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-4 mb-6">
        <h1 className="text-xl font-semibold text-slate-800">使用监控</h1>
        <select
          value={days}
          onChange={(e) => setDays(parseInt(e.target.value, 10))}
          className="px-4 py-2 border border-slate-300 rounded-lg text-sm"
        >
          <option value={1}>最近 1 天</option>
          <option value={7}>最近 7 天</option>
          <option value={30}>最近 30 天</option>
          <option value={90}>最近 90 天</option>
        </select>
      </div>

      {error && <p className="mb-4 text-sm text-red-600 bg-red-50 p-3 rounded-xl">{error}</p>}

      {loading ? (
        <p className="text-slate-500 py-8">加载中...</p>
      ) : stats ? (
        <div className="space-y-6">
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <div className="bg-white border border-slate-200 rounded-xl p-4">
              <p className="text-sm text-slate-500">调用次数</p>
              <p className="text-2xl font-semibold text-slate-800 mt-1">{stats.total_calls.toLocaleString()}</p>
            </div>
            <div className="bg-white border border-slate-200 rounded-xl p-4">
              <p className="text-sm text-slate-500">Prompt Tokens</p>
              <p className="text-2xl font-semibold text-slate-800 mt-1">{stats.total_prompt_tokens.toLocaleString()}</p>
            </div>
            <div className="bg-white border border-slate-200 rounded-xl p-4">
              <p className="text-sm text-slate-500">Completion Tokens</p>
              <p className="text-2xl font-semibold text-slate-800 mt-1">{stats.total_completion_tokens.toLocaleString()}</p>
            </div>
          </div>

          {stats.by_model.length > 0 && (
            <div className="bg-white border border-slate-200 rounded-xl p-4">
              <h2 className="font-medium text-slate-800 mb-3">按模型</h2>
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-slate-200">
                      <th className="text-left py-2">模型</th>
                      <th className="text-right py-2">调用</th>
                      <th className="text-right py-2">Prompt</th>
                      <th className="text-right py-2">Completion</th>
                    </tr>
                  </thead>
                  <tbody>
                    {stats.by_model.map((m) => (
                      <tr key={m.model} className="border-b border-slate-100">
                        <td className="py-2 font-mono">{m.model}</td>
                        <td className="text-right py-2">{m.calls.toLocaleString()}</td>
                        <td className="text-right py-2">{m.prompt_tokens.toLocaleString()}</td>
                        <td className="text-right py-2">{m.completion_tokens.toLocaleString()}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {stats.by_user.length > 0 && (
            <div className="bg-white border border-slate-200 rounded-xl p-4">
              <h2 className="font-medium text-slate-800 mb-3">按用户</h2>
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-slate-200">
                      <th className="text-left py-2">用户</th>
                      <th className="text-right py-2">调用</th>
                      <th className="text-right py-2">Prompt</th>
                      <th className="text-right py-2">Completion</th>
                    </tr>
                  </thead>
                  <tbody>
                    {stats.by_user.map((u) => (
                      <tr key={u.user_id} className="border-b border-slate-100">
                        <td className="py-2">{u.email || u.user_id}</td>
                        <td className="text-right py-2">{u.calls.toLocaleString()}</td>
                        <td className="text-right py-2">{u.prompt_tokens.toLocaleString()}</td>
                        <td className="text-right py-2">{u.completion_tokens.toLocaleString()}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {stats.total_calls === 0 && (
            <p className="text-slate-500 py-8 text-center">暂无使用数据</p>
          )}
        </div>
      ) : null}
    </div>
  )
}

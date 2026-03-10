import { useState, useEffect } from 'react'
import { getAdminUsers, adminRechargeUser, getMe, type User, type UserWithInstances } from '../api'

export default function Energy() {
  const [users, setUsers] = useState<UserWithInstances[]>([])
  const [me, setMe] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [recharging, setRecharging] = useState<number | null>(null)
  const [amount, setAmount] = useState<Record<number, string>>({})

  const load = () => {
    setLoading(true)
    Promise.all([getAdminUsers(), getMe()])
      .then(([u, m]) => {
        setUsers(u)
        setMe(m)
      })
      .catch((err) => setError(err instanceof Error ? err.message : '加载失败'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    load()
  }, [])

  const handleRecharge = async (u: UserWithInstances) => {
    const val = amount[u.id]?.trim()
    const amt = parseInt(val ?? '', 10)
    if (!val || isNaN(amt) || amt <= 0) {
      setError('请输入有效金币数量')
      return
    }
    setRecharging(u.id)
    setError('')
    try {
      await adminRechargeUser(u.id, amt)
      setAmount((prev) => ({ ...prev, [u.id]: '' }))
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : '充值失败')
    } finally {
      setRecharging(null)
    }
  }

  return (
    <div className="max-w-4xl mx-auto">
      <h1 className="text-xl font-semibold text-slate-800 mb-4">金币管理</h1>
      <p className="text-sm text-slate-500 mb-4">为指定用户充值金币，用户可用金币领养宠物或喂养宠物恢复活力</p>

      {error && (
        <p className="mb-4 text-sm text-red-600 bg-red-50 p-3 rounded-xl">{error}</p>
      )}

      {loading ? (
        <p className="text-slate-500 py-8">加载中...</p>
      ) : (
        <div className="space-y-3">
          {users.map((u) => (
            <div
              key={u.id}
              className="bg-white border border-slate-200 rounded-xl p-4 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3"
            >
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium text-slate-800">{u.email}</span>
                  {u.id === me?.id && (
                    <span className="px-2 py-0.5 text-xs bg-amber-100 text-amber-800 rounded">当前</span>
                  )}
                  <span className="px-2 py-0.5 text-xs bg-slate-100 text-slate-600 rounded">{u.role}</span>
                </div>
                <p className="text-sm text-slate-500 mt-1">ID: {u.id} · 金币: {u.energy} · 实例: {u.instance_count ?? 0}</p>
              </div>
              <div className="flex gap-2 flex-shrink-0 items-center">
                <input
                  type="number"
                  min={1}
                  placeholder="充值数量"
                  value={amount[u.id] ?? ''}
                  onChange={(e) => setAmount((prev) => ({ ...prev, [u.id]: e.target.value }))}
                  className="w-24 px-3 py-2 border border-slate-300 rounded-lg text-sm"
                />
                <button
                  onClick={() => handleRecharge(u)}
                  disabled={!!recharging}
                  className="px-4 py-2 text-sm bg-slate-800 text-white rounded-lg active:bg-slate-700 disabled:opacity-50 min-h-[40px]"
                >
                  {recharging === u.id ? '充值中...' : '充值'}
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

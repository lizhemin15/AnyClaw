import { useState } from 'react'
import { redeemCode, type User } from '../api'

export default function Redeem({ user, onRedeem }: { user: User | null; onRedeem?: () => void }) {
  const [code, setCode] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState<number | null>(null)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    const trimmed = code.trim().toUpperCase()
    if (!trimmed) {
      setError('请输入激活码')
      return
    }
    setLoading(true)
    setError('')
    setSuccess(null)
    try {
      const res = await redeemCode(trimmed)
      setSuccess(res.energy)
      setCode('')
      onRedeem?.()
    } catch (err) {
      setError(err instanceof Error ? err.message : '兑换失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="max-w-md mx-auto">
      <div className="mb-6 p-4 sm:p-5 rounded-xl bg-gradient-to-br from-amber-50 to-orange-50 border-2 border-amber-200">
        <h1 className="text-2xl font-bold text-slate-800 mb-1">🎫 激活码兑换</h1>
        <p className="text-base text-slate-600">当前余额：<span className="font-bold text-amber-600">🪙 {user?.energy ?? 0}</span></p>
      </div>

      {success !== null && (
        <div className="mb-4 p-4 rounded-lg bg-emerald-50 border border-emerald-100 text-emerald-700 text-sm">
          兑换成功！已获得 {success} 金币 🎉
        </div>
      )}

      {error && (
        <div className="mb-4 p-4 rounded-lg bg-red-50 border border-red-100 text-red-700 text-sm">{error}</div>
      )}

      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label htmlFor="code" className="block text-sm font-medium text-slate-700 mb-2">激活码</label>
          <input
            id="code"
            type="text"
            value={code}
            onChange={(e) => setCode(e.target.value.toUpperCase())}
            placeholder="请输入激活码"
            className="w-full px-4 py-3 border border-slate-300 rounded-xl text-lg font-mono tracking-wider uppercase placeholder:normal-case placeholder:tracking-normal"
            disabled={loading}
            autoComplete="off"
          />
        </div>
        <button
          type="submit"
          disabled={loading}
          className="w-full py-3.5 bg-amber-500 text-white font-medium rounded-xl hover:bg-amber-600 active:bg-amber-700 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {loading ? '兑换中...' : '兑换'}
        </button>
      </form>

      <p className="mt-6 text-sm text-slate-500 text-center">
        激活码可从管理员或其他渠道获取，兑换后金币将立即到账
      </p>
    </div>
  )
}

import { useState, useEffect } from 'react'
import { getRechargePlans, type User, type PaymentPlan } from '../api'

export default function Recharge({ user }: { user: User | null; onRecharge?: () => void }) {
  const [plans, setPlans] = useState<PaymentPlan[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    getRechargePlans()
      .then(setPlans)
      .catch(() => setPlans([]))
      .finally(() => setLoading(false))
  }, [])

  const handleSaveImage = async () => {
    setSaving(true)
    try {
      const res = await fetch('/pay_compressed.png')
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = '微信支付二维码.png'
      a.style.display = 'none'
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
    } catch {
      window.open('/pay_compressed.png', '_blank')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="max-w-md mx-auto pb-6 sm:pb-4">
      <div className="mb-6 p-4 sm:p-5 rounded-xl bg-gradient-to-br from-amber-50 to-orange-50 border-2 border-amber-200">
        <h1 className="text-2xl font-bold text-slate-800 mb-1">🪙 充值金币</h1>
        <p className="text-base text-slate-600">当前余额：<span className="font-bold text-amber-600">{user?.energy ?? 0}</span> 金币</p>
      </div>

      <div className="mb-6 p-4 sm:p-5 bg-white rounded-xl border border-slate-200">
        <p className="text-sm font-medium text-slate-700 mb-3">推荐使用微信支付</p>
        <div className="flex flex-col items-center gap-3">
          <div className="relative">
            <img src="/pay_compressed.png" alt="微信支付二维码" className="max-w-[260px] sm:max-w-[240px] w-full rounded-lg select-none touch-manipulation" draggable={false} />
          </div>
          <button
            type="button"
            onClick={handleSaveImage}
            disabled={saving}
            className="w-full sm:w-auto min-h-[48px] px-6 py-3 sm:py-2.5 bg-emerald-600 text-white text-sm font-medium rounded-xl active:bg-emerald-700 disabled:opacity-60 touch-manipulation"
          >
            {saving ? '保存中...' : '保存图片'}
          </button>
        </div>
        <p className="text-xs text-slate-500 text-center mt-3">扫码付款，请务必在备注中填写您的注册邮箱，人工审核成功后金币自动到账</p>
      </div>

      {loading ? (
        <p className="text-slate-500 py-4">加载中...</p>
      ) : plans.length > 0 ? (
        <div className="mb-6 space-y-3">
          <p className="text-sm font-medium text-slate-700">充值档位参考</p>
          {plans.map((p) => (
            <div key={p.id} className="p-4 bg-white border border-slate-200 rounded-xl">
              <p className="font-medium text-slate-800">{p.name}</p>
              <p className="text-sm text-slate-600 mt-0.5">{p.benefits || `${p.energy} 金币`}</p>
              <p className="text-amber-600 font-medium mt-1">¥{(p.price_cny / 100).toFixed(2)}</p>
            </div>
          ))}
        </div>
      ) : null}

      <div className="p-4 bg-slate-50 rounded-xl border border-slate-200">
        <p className="text-sm font-medium text-slate-700 mb-2">任意金额充值</p>
        <p className="text-sm text-slate-600">支持任意金额充值（≥10 元），付款时请在备注中填写您的<strong>注册邮箱</strong>，管理员审核通过后金币将自动到账。</p>
      </div>
    </div>
  )
}

import { useState, useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'
import { getPaymentPlans, createPaymentOrder, getMe, type PaymentPlan, type User } from '../api'

export default function Recharge() {
  const [plans, setPlans] = useState<PaymentPlan[]>([])
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [paying, setPaying] = useState<string | null>(null)
  const [wechatQr, setWechatQr] = useState<string | null>(null)
  const [searchParams] = useSearchParams()
  const paid = searchParams.get('paid') === '1'

  useEffect(() => {
    Promise.all([getPaymentPlans(), getMe()])
      .then(([p, u]) => {
        setPlans(p)
        setUser(u)
      })
      .catch((err) => setError(err instanceof Error ? err.message : '加载失败'))
      .finally(() => setLoading(false))
  }, [])

  const handlePay = async (plan: PaymentPlan, channel: 'alipay' | 'wechat') => {
    setPaying(plan.id + '-' + channel)
    setError('')
    try {
      const res = await createPaymentOrder(plan.id, channel)
      if (channel === 'alipay' && res.pay_url) {
        window.location.href = res.pay_url
      } else if (channel === 'wechat' && res.code_url) {
        setWechatQr(res.code_url)
        setError('')
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : '创建订单失败')
    } finally {
      setPaying(null)
    }
  }

  if (loading) {
    return (
      <div className="max-w-md mx-auto py-8">
        <p className="text-slate-500 text-center">加载中...</p>
      </div>
    )
  }

  return (
    <div className="max-w-md mx-auto">
      <h1 className="text-xl font-semibold text-slate-800 mb-2">金币充值</h1>
      <p className="text-sm text-slate-500 mb-4">当前余额：🪙 {user?.energy ?? 0}</p>

      {paid && (
        <div className="mb-4 p-4 rounded-lg bg-emerald-50 border border-emerald-100 text-emerald-700 text-sm">
          支付成功！金币已到账，请刷新页面查看。
        </div>
      )}

      {error && (
        <div className="mb-4 p-4 rounded-lg bg-red-50 border border-red-100 text-red-700 text-sm">{error}</div>
      )}

      {wechatQr && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => setWechatQr(null)}>
          <div className="bg-white rounded-xl p-6 max-w-md mx-4 text-center" onClick={(e) => e.stopPropagation()}>
            <p className="text-slate-500 mb-4">请使用微信扫码支付</p>
            <img src={`https://api.qrserver.com/v1/create-qr-code/?size=200x200&data=${encodeURIComponent(wechatQr)}`} alt="支付二维码" className="mx-auto mb-4" />
            <button onClick={() => setWechatQr(null)} className="px-4 py-2 text-sm text-slate-600 hover:bg-slate-100 rounded-lg">
              关闭
            </button>
          </div>
        </div>
      )}

      {plans.length === 0 ? (
        <p className="text-slate-500 py-8 text-center">暂无可购买的充值档位，请联系管理员配置</p>
      ) : (
        <div className="space-y-4">
          {plans.map((p) => (
            <div
              key={p.id}
              className="bg-white border border-slate-200 rounded-xl p-4 flex items-center justify-between"
            >
              <div>
                <div className="font-medium text-slate-800">{p.name}</div>
                <div className="text-sm text-slate-500">¥{(p.price_cny / 100).toFixed(2)}</div>
              </div>
              <div className="flex gap-2">
                <button
                  onClick={() => handlePay(p, 'alipay')}
                  disabled={!!paying}
                  className="px-4 py-2 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50"
                >
                  {paying === p.id + '-alipay' ? '跳转中...' : '支付宝'}
                </button>
                <button
                  onClick={() => handlePay(p, 'wechat')}
                  disabled={!!paying}
                  className="px-4 py-2 text-sm bg-emerald-600 text-white rounded-lg hover:bg-emerald-700 disabled:opacity-50"
                >
                  {paying === p.id + '-wechat' ? '生成中...' : '微信'}
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

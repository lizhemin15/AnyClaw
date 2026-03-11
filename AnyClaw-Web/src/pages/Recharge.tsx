import { useState, useEffect } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { getPaymentPlans, createPaymentOrder, getMe, type PaymentPlan, type User } from '../api'

export default function Recharge() {
  const [plans, setPlans] = useState<PaymentPlan[]>([])
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [paying, setPaying] = useState<string | null>(null)
  const [wechatQr, setWechatQr] = useState<string | null>(null)
  const [alipayQr, setAlipayQr] = useState<string | null>(null)
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
    setAlipayQr(null)
    setWechatQr(null)
    try {
      const res = await createPaymentOrder(plan.id, channel)
      if (channel === 'alipay' && res.code_url) {
        setAlipayQr(res.code_url)
        setError('')
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
      <div className="mb-6 p-4 sm:p-5 rounded-xl bg-gradient-to-br from-amber-50 to-orange-50 border-2 border-amber-200 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-800 mb-1">💰 金币充值</h1>
          <p className="text-base text-slate-600">当前余额：<span className="font-bold text-amber-600">🪙 {user?.energy ?? 0}</span></p>
        </div>
        <Link to="/orders" className="text-sm text-slate-600 hover:text-slate-800">历史订单</Link>
      </div>

      {paid && (
        <div className="mb-4 p-4 rounded-lg bg-emerald-50 border border-emerald-100 text-emerald-700 text-sm">
          支付成功！金币已到账，请刷新页面查看。
        </div>
      )}

      {error && (
        <div className="mb-4 p-4 rounded-lg bg-red-50 border border-red-100 text-red-700 text-sm">{error}</div>
      )}

      {(wechatQr || alipayQr) && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => { setWechatQr(null); setAlipayQr(null) }}>
          <div className="bg-white rounded-xl p-6 max-w-md mx-4 text-center" onClick={(e) => e.stopPropagation()}>
            <p className="text-slate-500 mb-4">{alipayQr ? '请使用支付宝扫码支付' : '请使用微信扫码支付'}</p>
            {alipayQr && (alipayQr.startsWith('http://') || alipayQr.startsWith('https://')) && (
              <a
                href={alipayQr}
                className="block w-full mb-4 px-4 py-3 bg-blue-600 text-white rounded-xl font-medium hover:bg-blue-700 active:bg-blue-800"
              >
                打开支付宝完成支付
              </a>
            )}
            <img src={`https://api.qrserver.com/v1/create-qr-code/?size=200x200&data=${encodeURIComponent(alipayQr || wechatQr || '')}`} alt="支付二维码" className="mx-auto mb-4" />
            {alipayQr && (alipayQr.startsWith('http://') || alipayQr.startsWith('https://')) && (
              <p className="text-xs text-slate-400 mb-4">手机端可点击上方按钮直接跳转；电脑端请用支付宝扫码</p>
            )}
            <button onClick={() => { setWechatQr(null); setAlipayQr(null) }} className="px-4 py-2 text-sm text-slate-600 hover:bg-slate-100 rounded-lg">
              关闭
            </button>
          </div>
        </div>
      )}

      {plans.length === 0 ? (
        <p className="text-slate-500 py-8 text-center">暂无可购买的充值档位，请联系管理员配置</p>
      ) : (
        <div className="space-y-4">
          {plans.map((p, i) => {
            const discount = i === 0 ? 0 : i === 1 ? 10 : 20
            const origPrice = discount > 0 ? Math.round((p.price_cny / 100) / (1 - discount / 100) * 100) / 100 : p.price_cny / 100
            const isPopular = i === 1
            const isBest = i === 2
            return (
              <div
                key={p.id}
                className={`relative rounded-2xl overflow-hidden ${
                  isBest ? 'bg-gradient-to-br from-amber-50 to-orange-50 border-2 border-amber-200 shadow-lg' :
                  isPopular ? 'bg-gradient-to-br from-indigo-50 to-slate-50 border-2 border-indigo-200' :
                  'bg-white border border-slate-200'
                }`}
              >
                {isPopular && (
                  <div className="absolute top-0 right-0 bg-indigo-600 text-white text-xs font-medium px-3 py-1 rounded-bl-lg">
                    推荐
                  </div>
                )}
                {isBest && (
                  <div className="absolute top-0 right-0 bg-amber-500 text-white text-xs font-medium px-3 py-1 rounded-bl-lg">
                    最划算
                  </div>
                )}
                <div className="p-5 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="font-semibold text-slate-800 text-lg">{p.name}</span>
                      {discount > 0 && (
                        <span className="text-xs font-medium bg-emerald-500 text-white px-2 py-0.5 rounded-full">
                          {discount}% OFF
                        </span>
                      )}
                    </div>
                    <div className="mt-1 flex items-baseline gap-2">
                      <span className="text-xl font-bold text-slate-800">¥{(p.price_cny / 100).toFixed(2)}</span>
                      {discount > 0 && (
                        <span className="text-sm text-slate-400 line-through">¥{origPrice.toFixed(2)}</span>
                      )}
                    </div>
                    <div className="text-sm text-slate-500 mt-0.5">{p.energy} 金币 · 约 ¥{(p.price_cny / 100 / p.energy).toFixed(3)}/枚</div>
                  </div>
                  <div className="flex gap-2 flex-shrink-0">
                    <button
                      onClick={() => handlePay(p, 'alipay')}
                      disabled={!!paying}
                      className="px-4 py-2.5 text-sm bg-blue-600 text-white rounded-xl hover:bg-blue-700 disabled:opacity-50 font-medium"
                    >
                      {paying === p.id + '-alipay' ? '生成中...' : '支付宝'}
                    </button>
                    <button
                      onClick={() => handlePay(p, 'wechat')}
                      disabled={!!paying}
                      className="px-4 py-2.5 text-sm bg-emerald-600 text-white rounded-xl hover:bg-emerald-700 disabled:opacity-50 font-medium"
                    >
                      {paying === p.id + '-wechat' ? '生成中...' : '微信'}
                    </button>
                  </div>
                </div>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

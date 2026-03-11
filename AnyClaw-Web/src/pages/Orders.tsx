import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { getOrders, type Order, type User } from '../api'

const planNames: Record<string, string> = {
  'plan-1': '入门',
  'plan-2': '进阶',
  'plan-3': '尊享',
}

const channelNames: Record<string, string> = {
  alipay: '支付宝',
  wechat: '微信',
}

export default function Orders({ user }: { user: User | null }) {
  const [orders, setOrders] = useState<Order[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const isAdmin = user?.role === 'admin'

  useEffect(() => {
    getOrders()
      .then(setOrders)
      .catch((err) => setError(err instanceof Error ? err.message : '加载失败'))
      .finally(() => setLoading(false))
  }, [])

  if (loading) {
    return (
      <div className="max-w-2xl mx-auto py-8">
        <p className="text-slate-500 text-center">加载中...</p>
      </div>
    )
  }

  return (
    <div className="max-w-2xl mx-auto">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-xl font-semibold text-slate-800">历史订单</h1>
        <Link to="/recharge" className="text-sm text-amber-600 font-medium hover:text-amber-700">
          去充值
        </Link>
      </div>

      {error && (
        <div className="mb-4 p-4 rounded-lg bg-red-50 border border-red-100 text-red-700 text-sm">{error}</div>
      )}

      {orders.length === 0 ? (
        <div className="py-12 text-center text-slate-500">
          <p>暂无订单</p>
          <Link to="/recharge" className="mt-2 inline-block text-amber-600 hover:text-amber-700">
            去充值
          </Link>
        </div>
      ) : (
        <div className="space-y-3">
          {orders.map((o) => (
            <div
              key={o.id}
              className="bg-white rounded-xl border border-slate-200 p-4 flex flex-col sm:flex-row sm:items-center justify-between gap-3"
            >
              <div className="min-w-0">
                {isAdmin && o.user_email && (
                  <p className="text-xs text-slate-500 mb-1 truncate">{o.user_email}</p>
                )}
                <div className="flex items-center gap-2 flex-wrap">
                  <span className="font-medium text-slate-800">
                    {planNames[o.plan_id] || o.plan_id}
                  </span>
                  <span className="text-sm text-slate-500">
                    ¥{(o.price_cny / 100).toFixed(2)} · {o.energy} 金币
                  </span>
                  <span className="text-xs px-2 py-0.5 rounded-full bg-slate-100 text-slate-600">
                    {channelNames[o.channel] || o.channel}
                  </span>
                </div>
                <p className="text-xs text-slate-400 mt-1">
                  {o.created_at} · {o.out_trade_no}
                </p>
              </div>
              <div className="flex-shrink-0">
                <span
                  className={`inline-block px-2.5 py-1 text-xs font-medium rounded-lg ${
                    o.status === 'paid'
                      ? 'bg-emerald-100 text-emerald-700'
                      : 'bg-amber-100 text-amber-700'
                  }`}
                >
                  {o.status === 'paid' ? '已支付' : '待支付'}
                </span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

import { useState, useEffect } from 'react'
import { getAdminConfig, putAdminConfig, testChannelConfig, testSMTPConfig, type AdminConfig, type Channel, type SMTPConfig, type PaymentConfig, type PaymentPlan, type AlipayConfig, type WechatConfig, type EnergyConfig } from '../api'

function genId() {
  return 'c-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8)
}

function genModelId() {
  return 'm-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8)
}

/** 根据 SMTP 地址或邮箱域名返回推荐端口，无匹配则返回 null */
function getDefaultSmtpPort(host: string, user: string): number | null {
  const h = (host || '').toLowerCase().trim()
  const domain = (user || '').includes('@') ? (user.split('@')[1] || '').toLowerCase() : ''
  const hostMap: Record<string, number> = {
    'smtp.163.com': 465,
    'smtp.126.com': 465,
    'smtp.yeah.net': 465,
    'smtp.qq.com': 465,
    'smtp.mail.qq.com': 465,
    'smtp.gmail.com': 587,
    'smtp.google.com': 587,
    'smtp.office365.com': 587,
    'smtp.outlook.com': 587,
    'smtp.live.com': 587,
    'smtp.aliyun.com': 465,
    'smtp.sina.com': 587,
  }
  const domainMap: Record<string, number> = {
    '163.com': 465,
    '126.com': 465,
    'yeah.net': 465,
    'qq.com': 465,
    'gmail.com': 587,
    'googlemail.com': 587,
    'outlook.com': 587,
    'hotmail.com': 587,
    'live.com': 587,
    'aliyun.com': 465,
    'sina.com': 587,
  }
  if (h && hostMap[h]) return hostMap[h]
  if (domain && domainMap[domain]) return domainMap[domain]
  return null
}

export default function AdminConfig() {
  const [config, setConfig] = useState<AdminConfig | null>(null)
  const [form, setForm] = useState<AdminConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [addingChannel, setAddingChannel] = useState(false)
  const [newChannel, setNewChannel] = useState({ name: '', api_key: '', api_base: '', model: 'gpt-4o' })
  const [editingChannel, setEditingChannel] = useState<string | null>(null)
  const [testingChannel, setTestingChannel] = useState<string | null>(null)
  const [testResult, setTestResult] = useState<{ id: string; ok: boolean; message: string } | null>(null)
  const [testingNew, setTestingNew] = useState(false)
  const [newTestResult, setNewTestResult] = useState<{ ok: boolean; message: string } | null>(null)
  const [testingSMTP, setTestingSMTP] = useState(false)
  const [smtpTestResult, setSmtpTestResult] = useState<{ ok: boolean; message: string } | null>(null)

  useEffect(() => {
    getAdminConfig()
      .then((c) => {
        const channels = (Array.isArray(c.channels) ? c.channels : []).map((ch) => {
          const models = ch.models && ch.models.length > 0 ? ch.models : [{ id: genModelId(), name: 'gpt-4o', enabled: ch.enabled }]
          return { ...ch, models }
        })
        const smtp = c.smtp ? { ...c.smtp } : undefined
        const payment: PaymentConfig = c.payment
          ? { ...c.payment, plans: c.payment.plans || [], alipay: c.payment.alipay ? { ...c.payment.alipay } : undefined, wechat: c.payment.wechat ? { ...c.payment.wechat } : undefined }
          : { plans: [], alipay: { enabled: false, app_id: '', private_key: '', alipay_public_key: '', is_sandbox: false }, wechat: { enabled: false, app_id: '', mch_id: '', api_v3_key: '', serial_no: '', private_key: '' } }
        const energy: EnergyConfig = c.energy
          ? { ...c.energy }
          : { tokens_per_energy: 1000, adopt_cost: 100, daily_consume: 10, min_energy_for_task: 5, zero_days_to_delete: 3, invite_reward: 50, new_user_energy: 100, invite_commission_rate: 5 }
        setConfig({ channels, smtp, payment, energy })
        setForm({ channels: JSON.parse(JSON.stringify(channels)), smtp, payment: JSON.parse(JSON.stringify(payment)), energy: { ...energy } })
      })
      .catch((err) => setError(err instanceof Error ? err.message : '加载失败'))
      .finally(() => setLoading(false))
  }, [])

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!form || saving) return
    setSaving(true)
    setError('')
    setSuccess('')
    try {
      await putAdminConfig(form)
      setSuccess('保存成功，配置已立即生效')
      setConfig(form)
      setAddingChannel(false)
      setEditingChannel(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const addChannel = () => {
    if (!form || !newChannel.name.trim() || !newChannel.api_key.trim()) return
    setNewTestResult(null)
    const ch: Channel = {
      id: genId(),
      name: newChannel.name.trim(),
      api_key: newChannel.api_key.trim(),
      api_base: newChannel.api_base.trim() || 'https://api.openai.com/v1',
      enabled: true,
      models: [{ id: genModelId(), name: (newChannel.model || 'gpt-4o').trim() || 'gpt-4o', enabled: true }],
    }
    const prev = form.channels || []
    setForm({
      channels: [...prev.map((c) => ({ ...c, enabled: false, models: (c.models || []).map((m) => ({ ...m, enabled: false })) })), ch],
    })
    setNewChannel({ name: '', api_key: '', api_base: '', model: 'gpt-4o' })
    setAddingChannel(false)
  }

  const removeChannel = (id: string) => {
    if (!form) return
    setForm({ channels: (form.channels || []).filter((c) => c.id !== id) })
    setEditingChannel(null)
  }

  const updateModelName = (channelId: string, name: string) => {
    if (!form) return
    const modelName = (name || 'gpt-4o').trim()
    setForm({
      channels: (form.channels || []).map((c) =>
        c.id === channelId
          ? {
              ...c,
              models:
                (c.models || []).length > 0
                  ? (c.models || []).map((m, i) => (i === 0 ? { ...m, name: modelName } : m))
                  : [{ id: genModelId(), name: modelName, enabled: c.enabled }],
            }
          : c
      ),
    })
  }

  const updateChannel = (id: string, upd: Partial<Channel>) => {
    if (!form) return
    setForm({
      channels: (form.channels || []).map((c) => (c.id === id ? { ...c, ...upd } : c)),
    })
  }

  const setChannelEnabled = (id: string, enabled: boolean) => {
    if (!form) return
    const channels = form.channels || []
    setForm({
      channels: channels.map((c) => ({
        ...c,
        enabled: c.id === id ? enabled : (enabled ? false : c.enabled),
        models:
          c.id === id && enabled
            ? (c.models || []).map((m, i) => ({ ...m, enabled: i === 0 }))
            : (c.models || []),
      })),
    })
  }

  const handleTestNewChannel = async () => {
    if (!newChannel.api_key?.trim()) {
      setNewTestResult({ ok: false, message: '请先填写 API Key' })
      return
    }
    setTestingNew(true)
    setNewTestResult(null)
    try {
      const res = await testChannelConfig({
        api_base: newChannel.api_base?.trim() || 'https://api.openai.com/v1',
        api_key: newChannel.api_key.trim(),
        model: (newChannel.model || 'gpt-4o').trim(),
      })
      setNewTestResult({ ok: res.ok, message: res.message })
    } catch (err) {
      setNewTestResult({ ok: false, message: err instanceof Error ? err.message : '测试失败' })
    } finally {
      setTestingNew(false)
    }
  }

  const handleTestChannel = async (ch: Channel) => {
    const model = (ch.models || [])[0]?.name || 'gpt-4o'
    if (!ch.api_key?.trim()) {
      setTestResult({ id: ch.id, ok: false, message: '请先填写 API Key' })
      return
    }
    setTestingChannel(ch.id)
    setTestResult(null)
    try {
      const res = await testChannelConfig({
        api_base: ch.api_base?.trim() || 'https://api.openai.com/v1',
        api_key: ch.api_key.trim(),
        model,
      })
      setTestResult({ id: ch.id, ok: res.ok, message: res.message })
    } catch (err) {
      setTestResult({ id: ch.id, ok: false, message: err instanceof Error ? err.message : '测试失败' })
    } finally {
      setTestingChannel(null)
    }
  }

  const updateSmtp = (upd: Partial<SMTPConfig>) => {
    if (!form) return
    setForm({
      ...form,
      smtp: { host: '', port: 587, user: '', pass: '', from: '', ...form.smtp, ...upd },
    })
  }

  const updatePayment = (upd: Partial<PaymentConfig>) => {
    if (!form) return
    const prev = form.payment || { plans: [] }
    setForm({
      ...form,
      payment: { ...prev, ...upd, plans: upd.plans ?? prev.plans },
    })
  }

  const updateAlipay = (upd: Partial<AlipayConfig>) => {
    if (!form) return
    const prev = form.payment?.alipay || { enabled: false, app_id: '', private_key: '', alipay_public_key: '', is_sandbox: false }
    updatePayment({ alipay: { ...prev, ...upd } })
  }

  const updateWechat = (upd: Partial<WechatConfig>) => {
    if (!form) return
    const prev = form.payment?.wechat || { enabled: false, app_id: '', mch_id: '', api_v3_key: '', serial_no: '', private_key: '' }
    updatePayment({ wechat: { ...prev, ...upd } })
  }

  const addPlan = () => {
    if (!form) return
    const plans = form.payment?.plans || []
    const newPlan: PaymentPlan = { id: 'p-' + Date.now(), name: '100 金币', energy: 100, price_cny: 100, sort: plans.length }
    updatePayment({ plans: [...plans, newPlan] })
  }

  const updatePlan = (id: string, upd: Partial<PaymentPlan>) => {
    if (!form?.payment?.plans) return
    updatePayment({
      plans: form.payment.plans.map((p) => (p.id === id ? { ...p, ...upd } : p)),
    })
  }

  const removePlan = (id: string) => {
    if (!form?.payment?.plans) return
    updatePayment({ plans: form.payment.plans.filter((p) => p.id !== id) })
  }

  const updateEnergy = (upd: Partial<EnergyConfig>) => {
    if (!form) return
    setForm({
      ...form,
      energy: { ...form.energy!, ...upd },
    })
  }

  const handleTestSMTP = async () => {
    const s = form?.smtp
    if (!s?.host?.trim()) {
      setSmtpTestResult({ ok: false, message: '请先填写 SMTP 地址' })
      return
    }
    setTestingSMTP(true)
    setSmtpTestResult(null)
    try {
      const res = await testSMTPConfig({
        host: s.host.trim(),
        port: s.port || 587,
        user: s.user?.trim(),
        pass: s.pass?.trim(),
        from: s.from?.trim(),
      })
      setSmtpTestResult(res)
    } catch (err) {
      setSmtpTestResult({ ok: false, message: err instanceof Error ? err.message : '测试失败' })
    } finally {
      setTestingSMTP(false)
    }
  }

  const channels = form?.channels ?? []
  const enabledModel = channels.flatMap((c) => c.models || []).find((m) => m.enabled)

  if (loading) {
    return (
      <div className="max-w-5xl mx-auto">
        <div className="bg-white rounded-lg border border-slate-200 shadow-sm p-8">
          <div className="animate-pulse space-y-4">
            <div className="h-6 bg-slate-200 rounded w-1/4" />
            <div className="h-12 bg-slate-100 rounded" />
            <div className="h-12 bg-slate-100 rounded" />
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="max-w-5xl mx-auto">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-slate-800">AI配置</h1>
        <p className="text-sm text-slate-500 mt-1">
          添加渠道并配置 API，一次只能启用一个渠道。当前启用：{enabledModel ? enabledModel.name : '无'}。
        </p>
      </div>

      {error && (
        <div className="mb-4 p-4 rounded-lg bg-red-50 border border-red-100 text-red-700 text-sm">{error}</div>
      )}
      {success && (
        <div className="mb-4 p-4 rounded-lg bg-emerald-50 border border-emerald-100 text-emerald-700 text-sm">
          {success}
        </div>
      )}

      <form onSubmit={handleSave}>
        {/* 经济参数 */}
        <div className="mb-6 bg-white rounded-lg border border-slate-200 shadow-sm overflow-hidden">
          <div className="px-5 py-4 border-b border-slate-200">
            <h2 className="font-semibold text-slate-800">经济参数</h2>
            <p className="text-sm text-slate-500 mt-1">金币/活力相关参数，保存后即时生效</p>
          </div>
          <div className="px-5 py-4">
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-1">Token 消耗比例</label>
                <input
                  type="number"
                  min={1}
                  value={form?.energy?.tokens_per_energy ?? 1000}
                  onChange={(e) => updateEnergy({ tokens_per_energy: parseInt(e.target.value, 10) || 1000 })}
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                />
                <p className="text-xs text-slate-500 mt-0.5">每 N token 消耗 1 活力</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-1">领养消耗</label>
                <input
                  type="number"
                  min={0}
                  value={form?.energy?.adopt_cost ?? 100}
                  onChange={(e) => updateEnergy({ adopt_cost: parseInt(e.target.value, 10) || 0 })}
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                />
                <p className="text-xs text-slate-500 mt-0.5">金币</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-1">每日消耗</label>
                <input
                  type="number"
                  min={0}
                  value={form?.energy?.daily_consume ?? 10}
                  onChange={(e) => updateEnergy({ daily_consume: parseInt(e.target.value, 10) || 0 })}
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                />
                <p className="text-xs text-slate-500 mt-0.5">每只宠物/天</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-1">对话门槛</label>
                <input
                  type="number"
                  min={0}
                  value={form?.energy?.min_energy_for_task ?? 5}
                  onChange={(e) => updateEnergy({ min_energy_for_task: parseInt(e.target.value, 10) || 0 })}
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                />
                <p className="text-xs text-slate-500 mt-0.5">低于此活力无法对话</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-1">无活力删除</label>
                <input
                  type="number"
                  min={1}
                  value={form?.energy?.zero_days_to_delete ?? 3}
                  onChange={(e) => updateEnergy({ zero_days_to_delete: parseInt(e.target.value, 10) || 3 })}
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                />
                <p className="text-xs text-slate-500 mt-0.5">连续无活力天数后永久消失</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-1">邀请奖励</label>
                <input
                  type="number"
                  min={0}
                  value={form?.energy?.invite_reward ?? 50}
                  onChange={(e) => updateEnergy({ invite_reward: parseInt(e.target.value, 10) || 0 })}
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                />
                <p className="text-xs text-slate-500 mt-0.5">双方各得</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-1">新用户初始</label>
                <input
                  type="number"
                  min={0}
                  value={form?.energy?.new_user_energy ?? 100}
                  onChange={(e) => updateEnergy({ new_user_energy: parseInt(e.target.value, 10) || 0 })}
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                />
                <p className="text-xs text-slate-500 mt-0.5">金币</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-1">充值返利比例</label>
                <input
                  type="number"
                  min={0}
                  max={100}
                  value={form?.energy?.invite_commission_rate ?? 5}
                  onChange={(e) => updateEnergy({ invite_commission_rate: parseInt(e.target.value, 10) })}
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                />
                <p className="text-xs text-slate-500 mt-0.5">受邀用户充值时邀请人获得 %</p>
              </div>
            </div>
          </div>
        </div>

        {/* 添加渠道 */}
        <div className="mb-6 bg-white rounded-lg border border-slate-200 shadow-sm overflow-hidden">
          <div className="px-5 py-4 border-b border-slate-200 flex items-center justify-between">
            <h2 className="font-semibold text-slate-800">渠道列表</h2>
            {!addingChannel ? (
              <button
                type="button"
                onClick={() => {
                  setAddingChannel(true)
                  setNewChannel({ name: '', api_key: '', api_base: '', model: 'gpt-4o' })
                }}
                className="px-4 py-2 text-sm bg-indigo-600 text-white rounded-lg hover:bg-indigo-700"
              >
                + 添加渠道
              </button>
            ) : (
              <div className="flex gap-2 items-center flex-wrap">
                <input
                  type="text"
                  value={newChannel.name}
                  onChange={(e) => setNewChannel((p) => ({ ...p, name: e.target.value }))}
                  placeholder="渠道名称，如 OpenAI"
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-32"
                />
                <input
                  type="password"
                  value={newChannel.api_key}
                  onChange={(e) => setNewChannel((p) => ({ ...p, api_key: e.target.value }))}
                  placeholder="API Key"
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-40 font-mono"
                />
                <input
                  type="url"
                  value={newChannel.api_base}
                  onChange={(e) => setNewChannel((p) => ({ ...p, api_base: e.target.value }))}
                  placeholder="API Base（可选）"
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-56"
                />
                <input
                  type="text"
                  value={newChannel.model}
                  onChange={(e) => setNewChannel((p) => ({ ...p, model: e.target.value }))}
                  placeholder="模型，如 gpt-4o"
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-32 font-mono"
                />
                <button
                  type="button"
                  onClick={handleTestNewChannel}
                  disabled={testingNew || !newChannel.api_key?.trim()}
                  className="px-4 py-2 text-sm text-slate-600 hover:bg-slate-100 rounded-lg disabled:opacity-50"
                >
                  {testingNew ? '测试中...' : '测试连通性'}
                </button>
                <button type="button" onClick={addChannel} className="px-4 py-2 text-sm bg-indigo-600 text-white rounded-lg hover:bg-indigo-700">
                  添加
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setAddingChannel(false)
                    setNewTestResult(null)
                  }}
                  className="px-4 py-2 text-sm text-slate-600 hover:bg-slate-100 rounded-lg"
                >
                  取消
                </button>
                {newTestResult && (
                  <span
                    className={`text-xs px-2 py-1 rounded ${
                      newTestResult.ok ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700'
                    }`}
                  >
                    {newTestResult.ok ? '✓ ' : '✗ '}
                    {newTestResult.message}
                  </span>
                )}
              </div>
            )}
          </div>

          {/* 渠道列表 */}
          <div className="divide-y divide-slate-100">
            {channels.length === 0 ? (
              <div className="px-5 py-8 text-center text-slate-500 text-sm">暂无渠道，点击上方添加</div>
            ) : (
              channels.map((ch) => (
                <div key={ch.id} className="px-5 py-4">
                  <div className="flex items-center gap-4 flex-wrap">
                    <button
                      type="button"
                      role="switch"
                      aria-checked={ch.enabled}
                      onClick={() => setChannelEnabled(ch.id, !ch.enabled)}
                      className={`relative inline-flex h-5 w-9 shrink-0 rounded-full border-2 border-transparent transition-colors ${
                        ch.enabled ? 'bg-indigo-600' : 'bg-slate-200'
                      }`}
                    >
                      <span
                        className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition ${
                          ch.enabled ? 'translate-x-4' : 'translate-x-0.5'
                        }`}
                      />
                    </button>
                    <span className="font-medium text-slate-800 min-w-[100px]">{ch.name}</span>
                    <span className="text-xs text-slate-400">{ch.enabled ? '已启用' : '未启用'}</span>
                    {editingChannel === ch.id ? (
                      <div className="flex gap-2 flex-1 flex-wrap">
                        <input
                          type="password"
                          value={ch.api_key}
                          onChange={(e) => updateChannel(ch.id, { api_key: e.target.value })}
                          placeholder="API Key"
                          className="px-3 py-1.5 border border-slate-300 rounded text-sm font-mono w-40"
                        />
                        <input
                          type="url"
                          value={ch.api_base}
                          onChange={(e) => updateChannel(ch.id, { api_base: e.target.value })}
                          placeholder="API Base"
                          className="px-3 py-1.5 border border-slate-300 rounded text-sm w-48"
                        />
                        <input
                          type="text"
                          value={(ch.models || [])[0]?.name || 'gpt-4o'}
                          onChange={(e) => updateModelName(ch.id, e.target.value)}
                          placeholder="模型"
                          className="px-3 py-1.5 border border-slate-300 rounded text-sm font-mono w-32"
                        />
                        <button type="button" onClick={() => setEditingChannel(null)} className="text-sm text-slate-600">
                          完成
                        </button>
                      </div>
                    ) : (
                      <span className="text-sm text-slate-500 truncate max-w-[200px]">
                        {ch.api_key ? '****' + ch.api_key.slice(-4) : '—'} · {ch.api_base || '—'} · {(ch.models || [])[0]?.name || 'gpt-4o'}
                      </span>
                    )}
                    {editingChannel !== ch.id && (
                      <button
                        type="button"
                        onClick={() => setEditingChannel(ch.id)}
                        className="text-sm text-indigo-600 hover:text-indigo-700"
                      >
                        编辑
                      </button>
                    )}
                    <button
                      type="button"
                      onClick={() => handleTestChannel(ch)}
                      disabled={testingChannel === ch.id || !ch.api_key?.trim()}
                      className="text-sm text-slate-600 hover:text-slate-800 disabled:opacity-50"
                    >
                      {testingChannel === ch.id ? '测试中...' : '测试连通性'}
                    </button>
                    <button type="button" onClick={() => removeChannel(ch.id)} className="text-sm text-red-600 hover:text-red-700">
                      删除
                    </button>
                  </div>
                  {testResult?.id === ch.id && (
                    <div
                      className={`mt-2 text-xs px-3 py-1.5 rounded ${
                        testResult.ok ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700'
                      }`}
                    >
                      {testResult.ok ? '✓ ' : '✗ '}
                      {testResult.message}
                    </div>
                  )}
                </div>
              ))
            )}
          </div>
        </div>

        {/* SMTP 配置 */}
        <div className="mb-6 bg-white rounded-lg border border-slate-200 shadow-sm overflow-hidden">
          <div className="px-5 py-4 border-b border-slate-200">
            <h2 className="font-semibold text-slate-800">邮件服务（注册验证码）</h2>
            <p className="text-sm text-slate-500 mt-1">配置后用户注册需邮箱验证码，留空则无需验证</p>
          </div>
          <div className="px-5 py-4 space-y-4">
            <div className="flex gap-4 flex-wrap">
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-1">SMTP 地址</label>
                <input
                  type="text"
                  value={form?.smtp?.host ?? ''}
                  onChange={(e) => {
                    const host = e.target.value
                    const port = getDefaultSmtpPort(host, form?.smtp?.user ?? '')
                    updateSmtp(port != null ? { host, port } : { host })
                  }}
                  placeholder="smtp.example.com"
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-48"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-1">端口</label>
                <input
                  type="number"
                  value={form?.smtp?.port ?? 587}
                  onChange={(e) => updateSmtp({ port: parseInt(e.target.value, 10) || 587 })}
                  placeholder="587"
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-20"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-1">用户名</label>
                <input
                  type="text"
                  value={form?.smtp?.user ?? ''}
                  onChange={(e) => {
                    const user = e.target.value
                    const port = getDefaultSmtpPort(form?.smtp?.host ?? '', user)
                    updateSmtp(port != null ? { user, port } : { user })
                  }}
                  placeholder="user@example.com"
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-48"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-1">密码</label>
                <input
                  type="password"
                  value={form?.smtp?.pass ?? ''}
                  onChange={(e) => updateSmtp({ pass: e.target.value })}
                  placeholder="密码或授权码（163/QQ 用授权码）"
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-40 font-mono"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-1">发件人</label>
                <input
                  type="text"
                  value={form?.smtp?.from ?? ''}
                  onChange={(e) => updateSmtp({ from: e.target.value })}
                  placeholder="留空则用用户名；163/QQ 建议填邮箱账号"
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-48"
                />
              </div>
              <div className="flex items-end">
                <button
                  type="button"
                  onClick={handleTestSMTP}
                  disabled={testingSMTP || !form?.smtp?.host?.trim()}
                  className="px-4 py-2 text-sm text-slate-600 hover:bg-slate-100 rounded-lg disabled:opacity-50"
                >
                  {testingSMTP ? '测试中...' : '测试连通性'}
                </button>
              </div>
            </div>
            {smtpTestResult && (
              <div
                className={`text-xs px-3 py-1.5 rounded ${
                  smtpTestResult.ok ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700'
                }`}
              >
                {smtpTestResult.ok ? '✓ ' : '✗ '}
                {smtpTestResult.message}
              </div>
            )}
          </div>
        </div>

        {/* 支付配置 */}
        <div className="mb-6 bg-white rounded-lg border border-slate-200 shadow-sm overflow-hidden">
          <div className="px-5 py-4 border-b border-slate-200">
            <h2 className="font-semibold text-slate-800">支付渠道（金币充值）</h2>
            <p className="text-sm text-slate-500 mt-1">配置支付宝、微信商家支付，用户可购买金币</p>
          </div>
          <div className="px-5 py-4 space-y-6">
            {/* 充值档位 */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <h3 className="text-sm font-medium text-slate-700">充值档位</h3>
                <button type="button" onClick={addPlan} className="text-sm text-indigo-600 hover:text-indigo-700">
                  + 添加档位
                </button>
              </div>
              <div className="space-y-2">
                {(form?.payment?.plans || []).map((p) => (
                  <div key={p.id} className="flex gap-2 items-center flex-wrap">
                    <input
                      type="text"
                      value={p.name}
                      onChange={(e) => updatePlan(p.id, { name: e.target.value })}
                      placeholder="名称"
                      className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-28"
                    />
                    <input
                      type="number"
                      value={p.energy}
                      onChange={(e) => updatePlan(p.id, { energy: parseInt(e.target.value, 10) || 0 })}
                      placeholder="金币"
                      className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-20"
                    />
                    <span className="text-slate-500 text-sm">金币</span>
                    <input
                      type="number"
                      value={p.price_cny}
                      onChange={(e) => updatePlan(p.id, { price_cny: parseInt(e.target.value, 10) || 0 })}
                      placeholder="价格(分)"
                      className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-24"
                    />
                    <span className="text-slate-500 text-sm">分 (¥{(p.price_cny / 100).toFixed(2)})</span>
                    <button type="button" onClick={() => removePlan(p.id)} className="text-sm text-red-600 hover:text-red-700">
                      删除
                    </button>
                  </div>
                ))}
              </div>
            </div>
            {/* 支付宝 */}
            <div>
              <h3 className="text-sm font-medium text-slate-700 mb-2">支付宝</h3>
              <div className="flex items-center gap-2 mb-2">
                <input type="checkbox" checked={form?.payment?.alipay?.enabled ?? false} onChange={(e) => updateAlipay({ enabled: e.target.checked })} />
                <span className="text-sm">启用</span>
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs text-slate-500 mb-1">AppID</label>
                  <input
                    type="text"
                    value={form?.payment?.alipay?.app_id ?? ''}
                    onChange={(e) => updateAlipay({ app_id: e.target.value })}
                    placeholder="应用 AppID"
                    className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                  />
                </div>
                <div>
                  <label className="block text-xs text-slate-500 mb-1">应用私钥 (PEM)</label>
                  <input
                    type="password"
                    value={form?.payment?.alipay?.private_key ?? ''}
                    onChange={(e) => updateAlipay({ private_key: e.target.value })}
                    placeholder="应用私钥"
                    className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full font-mono"
                  />
                </div>
                <div>
                  <label className="block text-xs text-slate-500 mb-1">支付宝公钥</label>
                  <input
                    type="password"
                    value={form?.payment?.alipay?.alipay_public_key ?? ''}
                    onChange={(e) => updateAlipay({ alipay_public_key: e.target.value })}
                    placeholder="支付宝公钥"
                    className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full font-mono"
                  />
                </div>
                <div className="flex items-center gap-2">
                  <input type="checkbox" checked={form?.payment?.alipay?.is_sandbox ?? false} onChange={(e) => updateAlipay({ is_sandbox: e.target.checked })} />
                  <span className="text-sm">沙箱环境</span>
                </div>
              </div>
            </div>
            {/* 微信支付 */}
            <div>
              <h3 className="text-sm font-medium text-slate-700 mb-2">微信支付</h3>
              <div className="flex items-center gap-2 mb-2">
                <input type="checkbox" checked={form?.payment?.wechat?.enabled ?? false} onChange={(e) => updateWechat({ enabled: e.target.checked })} />
                <span className="text-sm">启用</span>
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs text-slate-500 mb-1">AppID</label>
                  <input
                    type="text"
                    value={form?.payment?.wechat?.app_id ?? ''}
                    onChange={(e) => updateWechat({ app_id: e.target.value })}
                    placeholder="应用 AppID"
                    className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                  />
                </div>
                <div>
                  <label className="block text-xs text-slate-500 mb-1">商户号 (MchID)</label>
                  <input
                    type="text"
                    value={form?.payment?.wechat?.mch_id ?? ''}
                    onChange={(e) => updateWechat({ mch_id: e.target.value })}
                    placeholder="商户号"
                    className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                  />
                </div>
                <div>
                  <label className="block text-xs text-slate-500 mb-1">APIv3 密钥</label>
                  <input
                    type="password"
                    value={form?.payment?.wechat?.api_v3_key ?? ''}
                    onChange={(e) => updateWechat({ api_v3_key: e.target.value })}
                    placeholder="APIv3 密钥"
                    className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full font-mono"
                  />
                </div>
                <div>
                  <label className="block text-xs text-slate-500 mb-1">证书序列号</label>
                  <input
                    type="text"
                    value={form?.payment?.wechat?.serial_no ?? ''}
                    onChange={(e) => updateWechat({ serial_no: e.target.value })}
                    placeholder="证书序列号"
                    className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                  />
                </div>
                <div className="sm:col-span-2">
                  <label className="block text-xs text-slate-500 mb-1">商户私钥 (PEM)</label>
                  <input
                    type="password"
                    value={form?.payment?.wechat?.private_key ?? ''}
                    onChange={(e) => updateWechat({ private_key: e.target.value })}
                    placeholder="apiclient_key.pem 内容"
                    className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full font-mono"
                  />
                </div>
              </div>
            </div>
          </div>
        </div>

        <div className="flex justify-end gap-2">
          {JSON.stringify(form) !== JSON.stringify(config) && (
            <button
              type="button"
              onClick={() => setForm(config ? JSON.parse(JSON.stringify(config)) : form)}
              className="px-4 py-2 text-sm border border-slate-300 rounded-lg text-slate-700 hover:bg-slate-100"
            >
              重置
            </button>
          )}
          <button type="submit" disabled={saving} className="px-4 py-2 text-sm bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 disabled:opacity-50">
            {saving ? '保存中...' : '保存'}
          </button>
        </div>
      </form>
    </div>
  )
}

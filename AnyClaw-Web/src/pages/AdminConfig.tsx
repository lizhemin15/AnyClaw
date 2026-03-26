import { useState, useEffect, useCallback } from 'react'
import { getAdminConfig, putAdminConfig, getChannelStatus, setUsageCorrection, testChannelConfig, testSMTPConfig, testVoiceAPIConfig, adminReconnectInstances, type AdminConfig, type Channel, type ChannelStatus, type SMTPConfig, type PaymentConfig, type PaymentPlan, type EnergyConfig, type ContainerConfig, type COSConfig, type VoiceAPIEndpoint } from '../api'
import { useUnsavedConfig } from '../contexts/UnsavedConfigContext'

type ChannelTestKind = 'text' | 'image' | 'video'

function genId() {
  return 'c-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8)
}

function genModelId() {
  return 'm-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8)
}

// ASR（语音识别）支持 Whisper，Groq 与 ChatAnywhere 均可用
const ASR_PROVIDERS: { id: string; name: string; endpoint: string }[] = [
  { id: 'chatanywhere', name: 'ChatAnywhere', endpoint: 'https://api.chatanywhere.org/v1' },
  { id: 'groq',         name: 'Groq',         endpoint: 'https://api.groq.com/openai/v1'  },
]
// TTS（语音合成）Groq 不支持，Xiaomi MiMo 为小米 TTS
const TTS_PROVIDERS: { id: string; name: string; endpoint: string }[] = [
  { id: 'chatanywhere', name: 'ChatAnywhere', endpoint: 'https://api.chatanywhere.org/v1' },
  { id: 'xiaomi_mimo',  name: 'Xiaomi MiMo',  endpoint: 'https://api.xiaomimimo.com/v1' },
]

function getChannelProvider(ch: Channel): string {
  if (ch.name?.trim()) return ch.name.trim()
  const base = (ch.api_base || 'https://api.openai.com/v1').trim().replace(/\/$/, '')
  return base || 'https://api.openai.com/v1'
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
  const [newChannel, setNewChannel] = useState({ name: '', api_key: '', api_base: '', model: 'gpt-4o', daily_tokens_limit: 0, qps_limit: 0 })
  const [editingChannel, setEditingChannel] = useState<string | null>(null)
  const [channelTestBusy, setChannelTestBusy] = useState<{ id: string; kind: ChannelTestKind } | null>(null)
  const [testResult, setTestResult] = useState<{ id: string; kind: ChannelTestKind; ok: boolean; message: string } | null>(null)
  const [testingNew, setTestingNew] = useState<ChannelTestKind | null>(null)
  const [newTestResult, setNewTestResult] = useState<{ ok: boolean; message: string; kind: ChannelTestKind } | null>(null)
  const [testingSMTP, setTestingSMTP] = useState(false)
  const [smtpTestResult, setSmtpTestResult] = useState<{ ok: boolean; message: string } | null>(null)
  const [reconnecting, setReconnecting] = useState(false)
  const [reconnectResult, setReconnectResult] = useState<{ ok: boolean; message: string; reconnected?: number } | null>(null)
  const [channelStatus, setChannelStatus] = useState<Record<string, ChannelStatus>>({})
  const [correctingChannel, setCorrectingChannel] = useState<Channel | null>(null)
  const [correctedTotal, setCorrectedTotal] = useState('')
  const [correcting, setCorrecting] = useState(false)
  const [testingVoiceEndpoint, setTestingVoiceEndpoint] = useState<string | null>(null)
  const [voiceTestResult, setVoiceTestResult] = useState<{ id: string; ok: boolean; message: string; latency?: number } | null>(null)
  const [voiceApiStatus, setVoiceApiStatus] = useState<Record<string, ChannelStatus>>({})

  const unsavedCtx = useUnsavedConfig()
  const hasUnsaved = !!(form && config && JSON.stringify(form) !== JSON.stringify(config))

  useEffect(() => {
    if (unsavedCtx) unsavedCtx.setHasUnsaved(hasUnsaved)
  }, [hasUnsaved, unsavedCtx])

  useEffect(() => {
    if (!unsavedCtx) return
    const h = (e: BeforeUnloadEvent) => {
      if (hasUnsaved) e.preventDefault()
    }
    window.addEventListener('beforeunload', h)
    return () => window.removeEventListener('beforeunload', h)
  }, [hasUnsaved, unsavedCtx])

  const doSave = useCallback(async () => {
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
      throw err
    } finally {
      setSaving(false)
    }
  }, [form, saving])

  useEffect(() => {
    if (!unsavedCtx) return
    unsavedCtx.registerSaveHandler(hasUnsaved ? doSave : null)
    return () => {
      unsavedCtx.registerSaveHandler(null)
      unsavedCtx.setHasUnsaved(false)
    }
  }, [unsavedCtx, hasUnsaved, doSave])

  useEffect(() => {
    getAdminConfig()
      .then((c) => {
        const channels = (Array.isArray(c.channels) ? c.channels : []).map((ch) => {
          const models = ch.models && ch.models.length > 0 ? ch.models : [{ id: genModelId(), name: 'gpt-4o', enabled: ch.enabled }]
          return { ...ch, models }
        })
        const smtp = c.smtp ? { ...c.smtp } : undefined
        const defPlans: PaymentPlan[] = [
          { id: 'plan-1', name: '入门', benefits: '100 金币', energy: 100, price_cny: 100, sort: 0 },
          { id: 'plan-2', name: '进阶', benefits: '500 金币', energy: 500, price_cny: 450, sort: 1 },
          { id: 'plan-3', name: '尊享', benefits: '2000 金币', energy: 2000, price_cny: 1600, sort: 2 },
        ]
        const rawPlans = c.payment?.plans || []
        const plans = [0, 1, 2].map((i) => rawPlans[i] || defPlans[i])
        const defYg = { wechat: { enabled: false, mch_id: '', key: '' }, alipay: { enabled: false, mch_id: '', key: '' } }
        const yg = c.payment?.yungouos
        const payment: PaymentConfig = c.payment
          ? { ...c.payment, plans, yungouos: { wechat: yg?.wechat ? { ...yg.wechat } : defYg.wechat, alipay: yg?.alipay ? { ...yg.alipay } : defYg.alipay } }
          : { plans: defPlans, yungouos: defYg }
        const defaultEnergy: EnergyConfig = { tokens_per_energy: 1000, adopt_cost: 100, daily_consume: 0, min_energy_for_task: 5, zero_days_to_delete: 0, invite_reward: 50, new_user_energy: 0, daily_login_bonus: 10, invite_commission_rate: 5, monthly_subscription_cost: 50 }
        const energy: EnergyConfig = c.energy ? { ...defaultEnergy, ...c.energy } : defaultEnergy
        const container: ContainerConfig = c.container ? { ...c.container } : { workspace_size_gb: 0 }
        const cos: COSConfig | undefined = (c as { cos?: COSConfig }).cos
        const api_url = (c as { api_url?: string }).api_url ?? ''
        const voice_api: VoiceAPIEndpoint[] = Array.isArray((c as { voice_api?: VoiceAPIEndpoint[] }).voice_api) ? (c as { voice_api?: VoiceAPIEndpoint[] }).voice_api! : []
        const tts_api: VoiceAPIEndpoint[] = Array.isArray((c as { tts_api?: VoiceAPIEndpoint[] }).tts_api) ? (c as { tts_api?: VoiceAPIEndpoint[] }).tts_api! : []
        setConfig({ channels, smtp, payment, energy, container, cos, api_url, voice_api, tts_api })
        setForm({ channels: JSON.parse(JSON.stringify(channels)), smtp, payment: JSON.parse(JSON.stringify(payment)), energy: { ...energy }, container: { ...container }, cos: cos ? { ...cos } : undefined, api_url, voice_api: JSON.parse(JSON.stringify(voice_api)), tts_api: JSON.parse(JSON.stringify(tts_api)) })
      })
      .catch((err) => setError(err instanceof Error ? err.message : '加载失败'))
      .finally(() => setLoading(false))
  }, [])

  const refreshChannelStatus = useCallback(async () => {
    try {
      const data = await getChannelStatus()
      const map: Record<string, ChannelStatus> = {}
      for (const s of data.status) map[s.channel_id] = s
      setChannelStatus(map)
      const apiMap: Record<string, ChannelStatus> = {}
      if (data.voice_api_status) {
        for (const s of data.voice_api_status) apiMap[s.channel_id] = s
      }
      setVoiceApiStatus(apiMap)
    } catch {
      // 静默失败
    }
  }, [])

  useEffect(() => {
    const count = (form?.channels?.length ?? 0) + (form?.voice_api?.length ?? 0) + (form?.tts_api?.length ?? 0)
    if (count === 0) return
    refreshChannelStatus()
    const t = setInterval(refreshChannelStatus, 30000)
    return () => clearInterval(t)
  }, [form?.channels?.length, form?.voice_api?.length, form?.tts_api?.length, refreshChannelStatus])

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      await doSave()
    } catch {
      // doSave 已处理 setError
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
      daily_tokens_limit: newChannel.daily_tokens_limit ?? 0,
      qps_limit: newChannel.qps_limit ?? 0,
    }
    const prev = form.channels || []
    setForm({
      ...form,
      channels: [...prev, ch],
    })
    setNewChannel({ name: '', api_key: '', api_base: '', model: 'gpt-4o', daily_tokens_limit: 0, qps_limit: 0 })
    setAddingChannel(false)
  }

  const removeChannel = (id: string) => {
    if (!form) return
    setForm({ ...form, channels: (form.channels || []).filter((c) => c.id !== id) })
    setEditingChannel(null)
  }

  const updateModelName = (channelId: string, name: string) => {
    if (!form) return
    const modelName = (name || 'gpt-4o').trim()
    setForm({
      ...form,
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
      ...form,
      channels: (form.channels || []).map((c) => (c.id === id ? { ...c, ...upd } : c)),
    })
  }

  const setChannelEnabled = (id: string, enabled: boolean) => {
    if (!form) return
    const channels = form.channels || []
    setForm({
      ...form,
      channels: channels.map((c) => ({
        ...c,
        enabled: c.id === id ? enabled : c.enabled,
        models:
          c.id === id && enabled
            ? (c.models || []).map((m, i) => ({ ...m, enabled: i === 0 }))
            : (c.models || []),
      })),
    })
  }

  const handleTestNewChannel = async (kind: ChannelTestKind) => {
    if (!newChannel.api_key?.trim()) {
      setNewTestResult({ ok: false, message: '请先填写 API Key', kind })
      return
    }
    setTestingNew(kind)
    setNewTestResult(null)
    try {
      const basePayload = {
        api_base: newChannel.api_base?.trim() || 'https://api.openai.com/v1',
        api_key: newChannel.api_key.trim(),
        model: (newChannel.model || 'gpt-4o').trim(),
      }
      const res = await testChannelConfig(
        kind === 'text' ? basePayload : { ...basePayload, multimodal: kind }
      )
      setNewTestResult({ ok: res.ok, message: res.message, kind })
    } catch (err) {
      setNewTestResult({ ok: false, message: err instanceof Error ? err.message : '测试失败', kind })
    } finally {
      setTestingNew(null)
    }
  }

  const handleTestChannel = async (ch: Channel, kind: ChannelTestKind) => {
    const model = (ch.models || [])[0]?.name || 'gpt-4o'
    const apiKeyMasked = (ch.api_key?.trim() || '').startsWith('****')
    if (!ch.api_key?.trim()) {
      setTestResult({ id: ch.id, kind, ok: false, message: '请先填写 API Key' })
      return
    }
    setChannelTestBusy({ id: ch.id, kind })
    setTestResult(null)
    try {
      const mm = kind === 'text' ? undefined : kind
      const res = await testChannelConfig(
        apiKeyMasked
          ? { channel_id: ch.id, model, ...(mm ? { multimodal: mm } : {}) }
          : {
              channel_id: ch.id,
              api_base: ch.api_base?.trim() || 'https://api.openai.com/v1',
              api_key: ch.api_key.trim(),
              model,
              ...(mm ? { multimodal: mm } : {}),
            }
      )
      setTestResult({ id: ch.id, kind, ok: res.ok, message: res.message })
    } catch (err) {
      setTestResult({ id: ch.id, kind, ok: false, message: err instanceof Error ? err.message : '测试失败' })
    } finally {
      setChannelTestBusy(null)
    }
  }

  const channelTestLabel = (k: ChannelTestKind) =>
    k === 'text' ? '连通性' : k === 'image' ? '多模态·图' : '多模态·视频'

  const updateSmtp = (upd: Partial<SMTPConfig>) => {
    if (!form) return
    setForm({
      ...form,
      smtp: { host: '', port: 587, user: '', pass: '', from: '', ...form.smtp, ...upd },
    })
  }

  const updateCOS = (upd: Partial<COSConfig>) => {
    if (!form) return
    const cos = form.cos ?? { enabled: false, secret_id: '', secret_key: '', bucket: '', region: '', domain: '', path_prefix: 'media/' }
    setForm({ ...form, cos: { ...cos, ...upd } })
  }

  const updateContainer = (upd: Partial<ContainerConfig>) => {
    if (!form) return
    setForm({
      ...form,
      container: { ...(form.container ?? { workspace_size_gb: 0 }), ...upd },
    })
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
      <div className="mb-4 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-slate-800">AI配置</h1>
          <p className="text-sm text-slate-500 mt-1">
            添加渠道并配置 API，可同时启用多个渠道做负载均衡。默认模型：{enabledModel ? enabledModel.name : '无'}。
          </p>
        </div>
        <div className="flex gap-2 shrink-0">
          {JSON.stringify(form) !== JSON.stringify(config) && (
            <button
              type="button"
              onClick={() => setForm(config ? JSON.parse(JSON.stringify(config)) : form)}
              className="px-4 py-2 text-sm border border-slate-300 rounded-lg text-slate-700 hover:bg-slate-100"
            >
              重置
            </button>
          )}
          <button type="submit" form="admin-config-form" disabled={saving} className="px-4 py-2 text-sm bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 disabled:opacity-50 font-medium">
            {saving ? '保存中...' : '保存配置'}
          </button>
        </div>
      </div>

      {error && (
        <div className="mb-4 p-4 rounded-lg bg-red-50 border border-red-100 text-red-700 text-sm">{error}</div>
      )}
      {success && (
        <div className="mb-4 p-4 rounded-lg bg-emerald-50 border border-emerald-100 text-emerald-700 text-sm">
          {success}
        </div>
      )}

      <form id="admin-config-form" onSubmit={handleSave}>
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
                <label className="block text-sm font-medium text-slate-700 mb-1">招聘消耗</label>
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
                <label className="block text-sm font-medium text-slate-700 mb-1">新用户初始</label>
                <input
                  type="number"
                  min={0}
                  value={form?.energy?.new_user_energy ?? 0}
                  onChange={(e) => updateEnergy({ new_user_energy: parseInt(e.target.value, 10) || 0 })}
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                />
                <p className="text-xs text-slate-500 mt-0.5">金币</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-1">每日首次登录</label>
                <input
                  type="number"
                  min={0}
                  value={form?.energy?.daily_login_bonus ?? 10}
                  onChange={(e) => updateEnergy({ daily_login_bonus: parseInt(e.target.value, 10) || 0 })}
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                />
                <p className="text-xs text-slate-500 mt-0.5">金币</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-1">包月价格</label>
                <input
                  type="number"
                  min={0}
                  value={form?.energy?.monthly_subscription_cost ?? 50}
                  onChange={(e) => updateEnergy({ monthly_subscription_cost: parseInt(e.target.value, 10) || 0 })}
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                />
                <p className="text-xs text-slate-500 mt-0.5">金币/月，0 表示禁用包月</p>
              </div>
            </div>
          </div>
        </div>

        {/* 充值档位配置 */}
        <div className="mb-6 bg-white rounded-lg border border-slate-200 shadow-sm overflow-hidden">
          <div className="px-5 py-4 border-b border-slate-200">
            <h2 className="font-semibold text-slate-800">充值档位</h2>
            <p className="text-sm text-slate-500 mt-1">三档充值方案，名称与权益介绍将展示在用户充值页</p>
          </div>
          <div className="px-5 py-4">
            <div className="space-y-4">
              {[0, 1, 2].map((i) => (
                <div key={i} className="p-4 bg-slate-50 rounded-lg space-y-3">
                  <p className="text-sm font-medium text-slate-600">档位 {i + 1}</p>
                  <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
                    <div>
                      <label className="block text-xs text-slate-500 mb-0.5">名称</label>
                      <input
                        type="text"
                        value={form?.payment?.plans?.[i]?.name ?? ''}
                        onChange={(e) => {
                          if (!form) return
                          const plans = [...(form.payment?.plans || [])]
                          while (plans.length <= i) plans.push({ id: `plan-${i + 1}`, name: '', benefits: '', energy: 0, price_cny: 0, sort: i })
                          plans[i] = { ...plans[i], name: e.target.value }
                          setForm({ ...form, payment: { ...form.payment, plans } })
                        }}
                        placeholder="如：入门"
                        className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                      />
                    </div>
                    <div>
                      <label className="block text-xs text-slate-500 mb-0.5">权益介绍</label>
                      <input
                        type="text"
                        value={form?.payment?.plans?.[i]?.benefits ?? ''}
                        onChange={(e) => {
                          if (!form) return
                          const plans = [...(form.payment?.plans || [])]
                          while (plans.length <= i) plans.push({ id: `plan-${i + 1}`, name: '', benefits: '', energy: 0, price_cny: 0, sort: i })
                          plans[i] = { ...plans[i], benefits: e.target.value }
                          setForm({ ...form, payment: { ...form.payment, plans } })
                        }}
                        placeholder="如：100 金币"
                        className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                      />
                    </div>
                    <div>
                      <label className="block text-xs text-slate-500 mb-0.5">金币</label>
                      <input
                        type="number"
                        min={1}
                        value={form?.payment?.plans?.[i]?.energy ?? ''}
                        onChange={(e) => {
                          if (!form) return
                          const plans = [...(form.payment?.plans || [])]
                          while (plans.length <= i) plans.push({ id: `plan-${i + 1}`, name: '', benefits: '', energy: 0, price_cny: 0, sort: i })
                          plans[i] = { ...plans[i], energy: parseInt(e.target.value, 10) || 0 }
                          setForm({ ...form, payment: { ...form.payment, plans } })
                        }}
                        className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                      />
                    </div>
                    <div>
                      <label className="block text-xs text-slate-500 mb-0.5">价格（元）</label>
                      <input
                        type="number"
                        min={0.01}
                        step={0.01}
                        value={form?.payment?.plans?.[i]?.price_cny ? (form.payment.plans[i].price_cny / 100).toString() : ''}
                        onChange={(e) => {
                          if (!form) return
                          const plans = [...(form.payment?.plans || [])]
                          while (plans.length <= i) plans.push({ id: `plan-${i + 1}`, name: '', benefits: '', energy: 0, price_cny: 0, sort: i })
                          plans[i] = { ...plans[i], price_cny: Math.round((parseFloat(e.target.value) || 0) * 100) }
                          setForm({ ...form, payment: { ...form.payment, plans } })
                        }}
                        placeholder="如 9.9"
                        className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                      />
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* 部署配置 - API 地址与迁移 */}
        <div className="mb-6 bg-white rounded-lg border border-slate-200 shadow-sm overflow-hidden">
          <div className="px-5 py-4 border-b border-slate-200">
            <h2 className="font-semibold text-slate-800">部署配置</h2>
            <p className="text-sm text-slate-500 mt-1">API 地址供员工容器连接，迁移域名后需更新并重新连接</p>
          </div>
          <div className="px-5 py-4 space-y-4">
            <div>
              <label className="block text-sm font-medium text-slate-700 mb-1">API 地址</label>
              <input
                type="url"
                value={form?.api_url ?? ''}
                onChange={(e) => setForm((f) => (f ? { ...f, api_url: e.target.value } : f))}
                placeholder="https://your-domain.com"
                className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
              />
              <p className="text-xs text-slate-500 mt-1">迁移后在此更新新地址，保存后点击「重新连接实例」</p>
            </div>
            <div className="flex items-center gap-3">
              <button
                type="button"
                onClick={async () => {
                  setReconnecting(true)
                  setReconnectResult(null)
                  try {
                    const res = await adminReconnectInstances()
                    setReconnectResult(res)
                  } catch (err) {
                    setReconnectResult({ ok: false, message: err instanceof Error ? err.message : '操作失败' })
                  } finally {
                    setReconnecting(false)
                  }
                }}
                disabled={reconnecting}
                className="px-4 py-2 text-sm bg-amber-600 text-white rounded-lg hover:bg-amber-700 disabled:opacity-50"
              >
                {reconnecting ? '重新连接中...' : '重新连接实例'}
              </button>
              {reconnectResult && (
                <span className={`text-sm ${reconnectResult.ok ? 'text-emerald-600' : 'text-red-600'}`}>
                  {reconnectResult.ok ? `✓ ${reconnectResult.message}（${reconnectResult.reconnected ?? 0} 个）` : `✗ ${reconnectResult.message}`}
                </span>
              )}
            </div>
          </div>
        </div>

        {/* 容器存储 */}
        <div className="mb-6 bg-white rounded-lg border border-slate-200 shadow-sm overflow-hidden">
          <div className="px-5 py-4 border-b border-slate-200">
            <h2 className="font-semibold text-slate-800">容器存储</h2>
            <p className="text-sm text-slate-500 mt-1">每个员工实例工作区存储上限，0 表示不限制</p>
          </div>
          <div className="px-5 py-4">
            <div className="flex items-center gap-4">
              <div className="w-48">
                <label className="block text-sm font-medium text-slate-700 mb-1">工作区上限 (GB)</label>
                <input
                  type="number"
                  min={0}
                  max={1000}
                  value={form?.container?.workspace_size_gb ?? 0}
                  onChange={(e) => updateContainer({ workspace_size_gb: Math.max(0, parseInt(e.target.value, 10) || 0) })}
                  className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                />
              </div>
              <p className="text-sm text-slate-500 mt-6">0 = 不限制；新招聘实例生效</p>
            </div>
          </div>
        </div>

        {/* 腾讯云 COS */}
        <div className="mb-6 bg-white rounded-lg border border-slate-200 shadow-sm overflow-hidden">
          <div className="px-5 py-4 border-b border-slate-200">
            <h2 className="font-semibold text-slate-800">腾讯云 COS</h2>
            <p className="text-sm text-slate-500 mt-1">AI 发送的图片、音视频、文件将上传到 COS，消息中存链接，前端直接渲染或下载</p>
          </div>
          <div className="px-5 py-4 space-y-4">
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={form?.cos?.enabled ?? false}
                onChange={(e) => updateCOS({ enabled: e.target.checked })}
                className="rounded border-slate-300"
              />
              <span className="text-sm font-medium text-slate-700">启用 COS</span>
            </label>
            {form?.cos?.enabled && (
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-slate-700 mb-1">SecretId</label>
                  <input
                    type="password"
                    value={form.cos.secret_id ?? ''}
                    onChange={(e) => updateCOS({ secret_id: e.target.value })}
                    placeholder="腾讯云 API 密钥"
                    className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full font-mono"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-700 mb-1">SecretKey</label>
                  <input
                    type="password"
                    value={form.cos.secret_key ?? ''}
                    onChange={(e) => updateCOS({ secret_key: e.target.value })}
                    placeholder="腾讯云 API 密钥"
                    className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full font-mono"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-700 mb-1">Bucket</label>
                  <input
                    type="text"
                    value={form.cos.bucket ?? ''}
                    onChange={(e) => updateCOS({ bucket: e.target.value })}
                    placeholder="如 mybucket-1234567890"
                    className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-700 mb-1">Region</label>
                  <input
                    type="text"
                    value={form.cos.region ?? ''}
                    onChange={(e) => updateCOS({ region: e.target.value })}
                    placeholder="如 ap-guangzhou"
                    className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                  />
                </div>
                <div className="sm:col-span-2">
                  <label className="block text-sm font-medium text-slate-700 mb-1">自定义域名（可选）</label>
                  <input
                    type="url"
                    value={form.cos.domain ?? ''}
                    onChange={(e) => updateCOS({ domain: e.target.value })}
                    placeholder="https://cdn.example.com"
                    className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                  />
                  <p className="text-xs text-slate-500 mt-0.5">空则使用 COS 默认域名</p>
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-700 mb-1">路径前缀（可选）</label>
                  <input
                    type="text"
                    value={form.cos.path_prefix ?? 'media/'}
                    onChange={(e) => updateCOS({ path_prefix: e.target.value })}
                    placeholder="media/"
                    className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full"
                  />
                </div>
              </div>
            )}
          </div>
        </div>

        {/* 语音识别 (ASR) */}
        <div className="mb-6 bg-white rounded-lg border border-slate-200 shadow-sm overflow-hidden">
          <div className="px-5 py-4 border-b border-slate-200">
            <h2 className="font-semibold text-slate-800">语音识别 (ASR)</h2>
            <p className="text-xs text-slate-500 mt-0.5">填写 API Key 后启用，用于将语音转为文字。调度时优先使用第一个启用的端点。</p>
          </div>
          <div className="divide-y divide-slate-100">
            {ASR_PROVIDERS.map((provider) => {
              const ep = (form?.voice_api ?? []).find((e) => e.id === provider.id)
              const apiKey = ep?.api_key ?? ''
              const enabled = ep?.enabled ?? false
              const hasKey = !!apiKey.trim()
              const upsertProvider = (updates: Partial<VoiceAPIEndpoint>) => {
                if (!form) return
                const cur = form.voice_api ?? []
                const exists = cur.some((e) => e.id === provider.id)
                const next = exists ? cur.map((e) => e.id === provider.id ? { ...e, ...updates } : e) : [...cur, { id: provider.id, name: provider.name, endpoint: provider.endpoint, enabled: false, api_key: '', ...updates }]
                setForm({ ...form, voice_api: next })
              }
              return (
                <div key={'asr-' + provider.id} className="px-5 py-4">
                  <div className="flex items-center gap-4 flex-wrap">
                    <button type="button" role="switch" aria-checked={enabled} disabled={!hasKey} onClick={() => upsertProvider({ enabled: !enabled })}
                      className={`relative inline-flex h-5 w-9 shrink-0 rounded-full border-2 border-transparent transition-colors disabled:opacity-40 ${enabled && hasKey ? 'bg-indigo-600' : 'bg-slate-200'}`}>
                      <span className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition ${enabled && hasKey ? 'translate-x-4' : 'translate-x-0.5'}`} />
                    </button>
                    <div className="min-w-[120px]"><span className="font-medium text-slate-800">{provider.name}</span><p className="text-xs text-slate-400 font-mono">{provider.endpoint}</p></div>
                    {voiceApiStatus[provider.id] && hasKey && (
                      <span className="text-xs">{!enabled ? <span className="text-slate-500">手动关闭</span> : voiceApiStatus[provider.id].available ? <span className="text-emerald-600">可用</span> : <span className="text-amber-600">系统自动关闭</span>}{(voiceApiStatus[provider.id].in_flight ?? 0) > 0 && <span className="ml-2 text-indigo-500">进行中 {voiceApiStatus[provider.id].in_flight}</span>}</span>
                    )}
                    <input type="password" value={apiKey} onChange={(e) => upsertProvider({ api_key: e.target.value, enabled: !!e.target.value.trim() && enabled })} placeholder="API Key" className="px-3 py-1.5 border border-slate-300 rounded text-sm font-mono w-52" />
                    <button type="button" onClick={async () => {
                      if (!hasKey) { setVoiceTestResult({ id: provider.id, ok: false, message: '请先填写 API Key' }); return }
                      const masked = apiKey.startsWith('****')
                      setTestingVoiceEndpoint(provider.id); setVoiceTestResult(null)
                      try {
                        const res = await testVoiceAPIConfig(masked ? { endpoint_id: provider.id, api_type: 'voice' } : { endpoint_id: provider.id, api_type: 'voice', endpoint: provider.endpoint, api_key: apiKey.trim() })
                        setVoiceTestResult({ id: provider.id, ok: res.ok, message: res.message + (res.latency != null ? ` (${res.latency}ms)` : ''), latency: res.latency })
                      } catch (err) { setVoiceTestResult({ id: provider.id, ok: false, message: err instanceof Error ? err.message : '测试失败' }) }
                      finally { setTestingVoiceEndpoint(null) }
                    }} disabled={testingVoiceEndpoint === provider.id} className="text-sm text-slate-600 hover:text-slate-800 disabled:opacity-50">{testingVoiceEndpoint === provider.id ? '测试中...' : '测试'}</button>
                  </div>
                  {voiceTestResult?.id === provider.id && <div className={`mt-2 text-xs px-3 py-1.5 rounded ${voiceTestResult.ok ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700'}`}>{voiceTestResult.ok ? '✓ ' : '✗ '}{voiceTestResult.message}</div>}
                </div>
              )
            })}
          </div>
        </div>

        {/* 语音合成 (TTS) */}
        <div className="mb-6 bg-white rounded-lg border border-slate-200 shadow-sm overflow-hidden">
          <div className="px-5 py-4 border-b border-slate-200">
            <h2 className="font-semibold text-slate-800">语音合成 (TTS)</h2>
            <p className="text-xs text-slate-500 mt-0.5">填写 API Key 后启用，用于将文字转为语音。Groq 不支持 TTS，可配置 Xiaomi MiMo 或 ChatAnywhere。空则回退到 ASR 中非 Groq 的第一个。</p>
          </div>
          <div className="divide-y divide-slate-100">
            {TTS_PROVIDERS.map((provider) => {
              const ep = (form?.tts_api ?? []).find((e) => e.id === provider.id)
              const apiKey = ep?.api_key ?? ''
              const enabled = ep?.enabled ?? false
              const hasKey = !!apiKey.trim()
              const upsertProvider = (updates: Partial<VoiceAPIEndpoint>) => {
                if (!form) return
                const cur = form.tts_api ?? []
                const exists = cur.some((e) => e.id === provider.id)
                const next = exists ? cur.map((e) => e.id === provider.id ? { ...e, ...updates } : e) : [...cur, { id: provider.id, name: provider.name, endpoint: provider.endpoint, enabled: false, api_key: '', ...updates }]
                setForm({ ...form, tts_api: next })
              }
              return (
                <div key={'tts-' + provider.id} className="px-5 py-4">
                  <div className="flex items-center gap-4 flex-wrap">
                    <button type="button" role="switch" aria-checked={enabled} disabled={!hasKey} onClick={() => upsertProvider({ enabled: !enabled })}
                      className={`relative inline-flex h-5 w-9 shrink-0 rounded-full border-2 border-transparent transition-colors disabled:opacity-40 ${enabled && hasKey ? 'bg-indigo-600' : 'bg-slate-200'}`}>
                      <span className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition ${enabled && hasKey ? 'translate-x-4' : 'translate-x-0.5'}`} />
                    </button>
                    <div className="min-w-[120px]"><span className="font-medium text-slate-800">{provider.name}</span><p className="text-xs text-slate-400 font-mono">{provider.endpoint}</p></div>
                    <input type="password" value={apiKey} onChange={(e) => upsertProvider({ api_key: e.target.value, enabled: !!e.target.value.trim() && enabled })} placeholder="API Key" className="px-3 py-1.5 border border-slate-300 rounded text-sm font-mono w-52" />
                    <button type="button" onClick={async () => {
                      if (!hasKey) { setVoiceTestResult({ id: 'tts-' + provider.id, ok: false, message: '请先填写 API Key' }); return }
                      const masked = apiKey.startsWith('****')
                      setTestingVoiceEndpoint('tts-' + provider.id); setVoiceTestResult(null)
                      try {
                        const res = await testVoiceAPIConfig(masked ? { endpoint_id: provider.id, api_type: 'tts' } : { endpoint_id: provider.id, api_type: 'tts', endpoint: provider.endpoint, api_key: apiKey.trim() })
                        setVoiceTestResult({ id: 'tts-' + provider.id, ok: res.ok, message: res.message + (res.latency != null ? ` (${res.latency}ms)` : ''), latency: res.latency })
                      } catch (err) { setVoiceTestResult({ id: 'tts-' + provider.id, ok: false, message: err instanceof Error ? err.message : '测试失败' }) }
                      finally { setTestingVoiceEndpoint(null) }
                    }} disabled={testingVoiceEndpoint === 'tts-' + provider.id} className="text-sm text-slate-600 hover:text-slate-800 disabled:opacity-50">{testingVoiceEndpoint === 'tts-' + provider.id ? '测试中...' : '测试'}</button>
                  </div>
                  {voiceTestResult?.id === 'tts-' + provider.id && <div className={`mt-2 text-xs px-3 py-1.5 rounded ${voiceTestResult.ok ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700'}`}>{voiceTestResult.ok ? '✓ ' : '✗ '}{voiceTestResult.message}</div>}
                </div>
              )
            })}
          </div>
        </div>

        {/* AI 渠道配置 */}
        <div className="mb-6 bg-white rounded-lg border border-slate-200 shadow-sm overflow-hidden">
          <div className="px-5 py-4 border-b border-slate-200 flex items-center justify-between">
            <div>
              <h2 className="font-semibold text-slate-800">AI 渠道配置</h2>
              <p className="text-xs text-slate-500 mt-0.5">可配置渠道的日 tokens 上限、QPS 限制，调度器将自动避开超限渠道</p>
            </div>
            {!addingChannel ? (
              <button
                type="button"
                onClick={() => {
                  setAddingChannel(true)
                  setNewChannel({ name: '', api_key: '', api_base: '', model: 'gpt-4o', daily_tokens_limit: 0, qps_limit: 0 })
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
                <div className="flex flex-col">
                  <label className="text-xs text-slate-500 mb-0.5">日 tokens 上限</label>
                  <input
                    type="number"
                    min={0}
                    value={newChannel.daily_tokens_limit ?? 0}
                    onChange={(e) => setNewChannel((p) => ({ ...p, daily_tokens_limit: parseInt(e.target.value, 10) || 0 }))}
                    placeholder="0=不限制"
                    className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-28"
                  />
                </div>
                <div className="flex flex-col">
                  <label className="text-xs text-slate-500 mb-0.5">QPS 上限</label>
                  <input
                    type="number"
                    min={0}
                    step={0.1}
                    value={newChannel.qps_limit ?? 0}
                    onChange={(e) => setNewChannel((p) => ({ ...p, qps_limit: parseFloat(e.target.value) || 0 }))}
                    placeholder="0=不限制"
                    className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-24"
                  />
                </div>
                <button
                  type="button"
                  onClick={() => handleTestNewChannel('text')}
                  disabled={testingNew !== null || !newChannel.api_key?.trim()}
                  className="px-3 py-2 text-sm text-slate-600 hover:bg-slate-100 rounded-lg disabled:opacity-50"
                >
                  {testingNew === 'text' ? '测试中...' : '测试连通性'}
                </button>
                <button
                  type="button"
                  onClick={() => handleTestNewChannel('image')}
                  disabled={testingNew !== null || !newChannel.api_key?.trim()}
                  className="px-3 py-2 text-sm text-slate-600 hover:bg-slate-100 rounded-lg disabled:opacity-50"
                  title="发送内嵌 base64 小图，校验 image_url 多模态"
                >
                  {testingNew === 'image' ? '测试中...' : '多模态·图'}
                </button>
                <button
                  type="button"
                  onClick={() => handleTestNewChannel('video')}
                  disabled={testingNew !== null || !newChannel.api_key?.trim()}
                  className="px-3 py-2 text-sm text-slate-600 hover:bg-slate-100 rounded-lg disabled:opacity-50"
                  title="仅 Moonshot API Base：上传内置短视频后 ms:// 调用"
                >
                  {testingNew === 'video' ? '测试中...' : '多模态·视频'}
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
                    [{channelTestLabel(newTestResult.kind)}] {newTestResult.ok ? '✓ ' : '✗ '}
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
                    {channelStatus[ch.id] ? (
                      <span className="flex items-center gap-3 text-xs flex-wrap">
                        {!ch.enabled ? (
                          <span className="text-slate-500">手动关闭</span>
                        ) : channelStatus[ch.id].available ? (
                          <span className="text-emerald-600">可用</span>
                        ) : (
                          <span className="text-amber-600">系统自动关闭</span>
                        )}
                        <span className="flex items-center gap-2 min-w-[160px]">
                          <span className="flex-1 min-w-[96px] max-w-[120px] h-2 bg-slate-200 rounded-full overflow-hidden">
                            <span
                              className={`block h-full rounded-full transition-all ${
                                (ch.daily_tokens_limit ?? 0) > 0
                                  ? (channelStatus[ch.id].token_usage_today ?? 0) >= (ch.daily_tokens_limit ?? 1)
                                    ? 'bg-red-500'
                                    : ((channelStatus[ch.id].token_usage_today ?? 0) / (ch.daily_tokens_limit ?? 1)) >= 0.8
                                      ? 'bg-amber-500'
                                      : 'bg-emerald-500'
                                  : 'bg-slate-400'
                              }`}
                              style={{
                                width: `${
                                  (() => {
                                    const used = channelStatus[ch.id].token_usage_today ?? 0
                                    const limit = ch.daily_tokens_limit ?? 0
                                    const pct =
                                      limit > 0
                                        ? (used / limit) * 100
                                        : (used / 50000) * 100
                                    return `${Math.max(pct > 0 ? 2 : 0, Math.min(100, pct))}%`
                                  })()
                                }`,
                              }}
                            />
                          </span>
                          <span className="text-slate-600 shrink-0">
                            {(channelStatus[ch.id].token_usage_today ?? 0).toLocaleString()}
                            {(ch.daily_tokens_limit ?? 0) > 0 && (
                              <span className="text-slate-400">/{(ch.daily_tokens_limit ?? 0).toLocaleString()}</span>
                            )}{' '}
                            tokens
                          </span>
                        </span>
                        {channelStatus[ch.id].in_flight > 0 && (
                          <span className="text-indigo-500">进行中 {channelStatus[ch.id].in_flight}</span>
                        )}
                        {channelStatus[ch.id].cooldown_until && (
                          <span className="text-amber-600">恢复 {channelStatus[ch.id].cooldown_until}</span>
                        )}
                        <button
                          type="button"
                          onClick={() => {
                            setCorrectingChannel(ch)
                            setCorrectedTotal(String(channelStatus[ch.id].token_usage_today ?? 0))
                          }}
                          className="text-xs text-indigo-600 hover:text-indigo-700"
                        >
                          矫正
                        </button>
                      </span>
                    ) : (
                      <span className="text-xs text-slate-400">{ch.enabled ? '已启用' : '手动关闭'}</span>
                    )}
                    {editingChannel === ch.id ? (
                      <div className="flex gap-2 flex-1 flex-wrap items-center">
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
                        <div className="flex flex-col">
                          <label className="text-xs text-slate-500 mb-0.5">日 tokens 上限</label>
                          <input
                            type="number"
                            min={0}
                            value={ch.daily_tokens_limit ?? 0}
                            onChange={(e) => updateChannel(ch.id, { daily_tokens_limit: parseInt(e.target.value, 10) || 0 })}
                            placeholder="0=不限制"
                            className="px-3 py-1.5 border border-slate-300 rounded text-sm w-28"
                          />
                        </div>
                        <div className="flex flex-col">
                          <label className="text-xs text-slate-500 mb-0.5">QPS 上限</label>
                          <input
                            type="number"
                            min={0}
                            step={0.1}
                            value={ch.qps_limit ?? 0}
                            onChange={(e) => updateChannel(ch.id, { qps_limit: parseFloat(e.target.value) || 0 })}
                            placeholder="0=不限制"
                            className="px-3 py-1.5 border border-slate-300 rounded text-sm w-24"
                          />
                        </div>
                        <button type="button" onClick={() => setEditingChannel(null)} className="text-sm text-slate-600">
                          完成
                        </button>
                      </div>
                    ) : (
                      <span className="text-sm text-slate-500 truncate max-w-[200px]">
                        {ch.api_key ? '****' + ch.api_key.slice(-4) : '—'} · {ch.api_base || '—'} · {(ch.models || [])[0]?.name || 'gpt-4o'}
                        {(ch.daily_tokens_limit ?? 0) > 0 && ` · 日限 ${(ch.daily_tokens_limit ?? 0).toLocaleString()} tokens`}
                        {(ch.qps_limit ?? 0) > 0 && ` · QPS ${ch.qps_limit}`}
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
                    <div className="flex flex-wrap gap-1 items-center">
                      <button
                        type="button"
                        onClick={() => handleTestChannel(ch, 'text')}
                        disabled={channelTestBusy?.id === ch.id || !ch.api_key?.trim()}
                        className="text-sm text-slate-600 hover:text-slate-800 disabled:opacity-50 px-1"
                      >
                        {channelTestBusy?.id === ch.id && channelTestBusy.kind === 'text' ? '测试中...' : '测试连通性'}
                      </button>
                      <button
                        type="button"
                        onClick={() => handleTestChannel(ch, 'image')}
                        disabled={channelTestBusy?.id === ch.id || !ch.api_key?.trim()}
                        className="text-sm text-slate-600 hover:text-slate-800 disabled:opacity-50 px-1"
                        title="base64 小图 + image_url"
                      >
                        {channelTestBusy?.id === ch.id && channelTestBusy.kind === 'image' ? '测试中...' : '多模态·图'}
                      </button>
                      <button
                        type="button"
                        onClick={() => handleTestChannel(ch, 'video')}
                        disabled={channelTestBusy?.id === ch.id || !ch.api_key?.trim()}
                        className="text-sm text-slate-600 hover:text-slate-800 disabled:opacity-50 px-1"
                        title="仅 Moonshot：内置短视频上传 + ms://"
                      >
                        {channelTestBusy?.id === ch.id && channelTestBusy.kind === 'video' ? '测试中...' : '多模态·视频'}
                      </button>
                    </div>
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
                      [{channelTestLabel(testResult.kind)}] {testResult.ok ? '✓ ' : '✗ '}
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

        <div className="flex justify-end gap-2 pt-4 border-t border-slate-200">
          <button type="submit" disabled={saving} className="px-4 py-2 text-sm bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 disabled:opacity-50">
            {saving ? '保存中...' : '保存配置'}
          </button>
        </div>
      </form>

      {correctingChannel && (
        <div className="fixed inset-0 bg-black/30 flex items-center justify-center z-50" onClick={() => setCorrectingChannel(null)}>
          <div className="bg-white rounded-lg shadow-xl p-5 max-w-sm w-full mx-4" onClick={(e) => e.stopPropagation()}>
            <h3 className="font-medium text-slate-800 mb-2">矫正今日 token 用量 · {correctingChannel.name || getChannelProvider(correctingChannel)}</h3>
            <p className="text-xs text-slate-500 mb-3">将今日显示值设为（系统会计算校正量）</p>
            <input
              type="number"
              min={0}
              value={correctedTotal}
              onChange={(e) => setCorrectedTotal(e.target.value)}
              placeholder="如 10000"
              className="px-3 py-2 border border-slate-300 rounded-lg text-sm w-full mb-4"
            />
            <div className="flex gap-2 justify-end">
              <button type="button" onClick={() => setCorrectingChannel(null)} className="px-4 py-2 text-sm text-slate-600 hover:bg-slate-100 rounded-lg">
                取消
              </button>
              <button
                type="button"
                disabled={correcting}
                onClick={async () => {
                  const val = parseInt(correctedTotal, 10)
                  if (isNaN(val) || val < 0) return
                  setCorrecting(true)
                  try {
                    await setUsageCorrection({
                      channel_id: correctingChannel.id,
                      corrected_total: val,
                    })
                    setCorrectingChannel(null)
                    refreshChannelStatus()
                  } catch (err) {
                    setError(err instanceof Error ? err.message : '矫正失败')
                  } finally {
                    setCorrecting(false)
                  }
                }}
                className="px-4 py-2 text-sm bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 disabled:opacity-50"
              >
                {correcting ? '提交中...' : '确定'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

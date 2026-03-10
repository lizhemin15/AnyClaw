import { useState, useEffect } from 'react'
import { getAdminConfig, putAdminConfig, testChannelConfig, type AdminConfig, type Channel } from '../api'

function genId() {
  return 'c-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8)
}

function genModelId() {
  return 'm-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8)
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

  useEffect(() => {
    getAdminConfig()
      .then((c) => {
        const channels = (Array.isArray(c.channels) ? c.channels : []).map((ch) => {
          const models = ch.models && ch.models.length > 0 ? ch.models : [{ id: genModelId(), name: 'gpt-4o', enabled: ch.enabled }]
          return { ...ch, models }
        })
        setConfig({ channels })
        setForm({ channels: JSON.parse(JSON.stringify(channels)) })
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
        <h1 className="text-xl font-semibold text-slate-800">渠道管理</h1>
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

import { useState, useEffect } from 'react'
import { getAdminConfig, putAdminConfig, type AdminConfig, type Channel, type ModelEntry } from '../api'

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
  const [newChannel, setNewChannel] = useState({ name: '', api_key: '', api_base: '' })
  const [editingChannel, setEditingChannel] = useState<string | null>(null)
  const [newModelByChannel, setNewModelByChannel] = useState<Record<string, string>>({})

  useEffect(() => {
    getAdminConfig()
      .then((c) => {
        const channels = Array.isArray(c.channels) ? c.channels : []
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
      setSuccess('保存成功，重启服务后生效')
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
    const ch: Channel = {
      id: genId(),
      name: newChannel.name.trim(),
      api_key: newChannel.api_key.trim(),
      api_base: newChannel.api_base.trim() || 'https://api.openai.com/v1',
      enabled: true,
      models: [],
    }
    setForm({ channels: [...(form.channels || []), ch] })
    setNewChannel({ name: '', api_key: '', api_base: '' })
    setAddingChannel(false)
  }

  const removeChannel = (id: string) => {
    if (!form) return
    setForm({ channels: (form.channels || []).filter((c) => c.id !== id) })
    setEditingChannel(null)
  }

  const updateChannel = (id: string, upd: Partial<Channel>) => {
    if (!form) return
    setForm({
      channels: (form.channels || []).map((c) => (c.id === id ? { ...c, ...upd } : c)),
    })
  }

  const setChannelEnabled = (id: string, enabled: boolean) => {
    updateChannel(id, { enabled })
  }

  const addModel = (channelId: string) => {
    const name = (newModelByChannel[channelId] || '').trim()
    if (!form || !name) return
    const channels = form.channels || []
    const ch = channels.find((c) => c.id === channelId)
    if (!ch || (ch.models || []).some((m) => m.name === name)) return
    const models = [...(ch.models || []), { id: genModelId(), name, enabled: false }]
    setForm({
      channels: channels.map((c) => (c.id === channelId ? { ...c, models } : c)),
    })
    setNewModelByChannel((prev) => ({ ...prev, [channelId]: '' }))
  }

  const removeModel = (channelId: string, modelId: string) => {
    if (!form) return
    const channels = form.channels || []
    const ch = channels.find((c) => c.id === channelId)
    if (!ch) return
    const hadEnabled = (ch.models || []).find((m) => m.id === modelId)?.enabled
    let models = (ch.models || []).filter((m) => m.id !== modelId)
    if (hadEnabled && models.length > 0 && !models.some((m) => m.enabled)) {
      models = models.map((m, i) => ({ ...m, enabled: i === 0 }))
    }
    setForm({
      channels: channels.map((c) => (c.id === channelId ? { ...c, models } : c)),
    })
  }

  const setModelEnabled = (channelId: string, modelId: string) => {
    if (!form) return
    setForm({
      channels: (form.channels || []).map((c) => ({
        ...c,
        models: (c.models || []).map((m) => ({
          ...m,
          enabled: c.id === channelId && m.id === modelId,
        })),
      })),
    })
  }

  const updateModelName = (channelId: string, modelId: string, name: string) => {
    if (!form) return
    setForm({
      channels: (form.channels || []).map((c) =>
        c.id === channelId
          ? { ...c, models: (c.models || []).map((m) => (m.id === modelId ? { ...m, name } : m)) }
          : c
      ),
    })
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
          添加渠道并配置 API，每个渠道可添加多个模型。一次只能启用一个模型作为新宠物默认。当前默认：{enabledModel ? enabledModel.name : '无（gpt-4o）'}
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
                onClick={() => setAddingChannel(true)}
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
                <button type="button" onClick={addChannel} className="px-4 py-2 text-sm bg-indigo-600 text-white rounded-lg hover:bg-indigo-700">
                  添加
                </button>
                <button type="button" onClick={() => setAddingChannel(false)} className="px-4 py-2 text-sm text-slate-600 hover:bg-slate-100 rounded-lg">
                  取消
                </button>
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
                        <button type="button" onClick={() => setEditingChannel(null)} className="text-sm text-slate-600">
                          完成
                        </button>
                      </div>
                    ) : (
                      <span className="text-sm text-slate-500 truncate max-w-[200px]">
                        {ch.api_key ? '****' + ch.api_key.slice(-4) : '—'} · {ch.api_base || '—'}
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
                    <button type="button" onClick={() => removeChannel(ch.id)} className="text-sm text-red-600 hover:text-red-700">
                      删除
                    </button>
                  </div>

                  {/* 模型列表 */}
                  <div className="mt-3 ml-14 pl-4 border-l-2 border-slate-100">
                    <div className="flex gap-2 items-center mb-2">
                      <input
                        type="text"
                        value={newModelByChannel[ch.id] || ''}
                        onChange={(e) => setNewModelByChannel((p) => ({ ...p, [ch.id]: e.target.value }))}
                        onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), addModel(ch.id))}
                        placeholder="添加模型，如 gpt-4o"
                        className="px-3 py-1.5 border border-slate-300 rounded text-sm font-mono w-40"
                      />
                      <button type="button" onClick={() => addModel(ch.id)} className="text-sm text-indigo-600 hover:text-indigo-700">
                        添加
                      </button>
                    </div>
                    <div className="space-y-1">
                      {(ch.models || []).map((m) => (
                        <div key={m.id} className="flex items-center gap-2">
                          <button
                            type="button"
                            role="radio"
                            aria-checked={m.enabled}
                            onClick={() => setModelEnabled(ch.id, m.id)}
                            className={`relative inline-flex h-5 w-9 shrink-0 rounded-full border-2 border-transparent transition-colors ${
                              m.enabled ? 'bg-indigo-600' : 'bg-slate-200'
                            }`}
                          >
                            <span
                              className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition ${
                                m.enabled ? 'translate-x-4' : 'translate-x-0.5'
                              }`}
                            />
                          </button>
                          <input
                            type="text"
                            value={m.name}
                            onChange={(e) => updateModelName(ch.id, m.id, e.target.value)}
                            className="px-2 py-1 border border-slate-200 rounded text-sm font-mono w-44"
                          />
                          <span className="text-xs text-slate-400">{m.enabled ? '默认' : ''}</span>
                          <button type="button" onClick={() => removeModel(ch.id, m.id)} className="text-xs text-red-500 hover:text-red-600">
                            删除
                          </button>
                        </div>
                      ))}
                    </div>
                  </div>
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

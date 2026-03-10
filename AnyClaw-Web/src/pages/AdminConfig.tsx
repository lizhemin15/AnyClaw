import { useState, useEffect } from 'react'
import { getAdminConfig, putAdminConfig, type AdminConfig } from '../api'

const PROVIDER_LABELS: Record<string, string> = {
  openai: 'OpenAI',
  anthropic: 'Anthropic Claude',
  openrouter: 'OpenRouter',
}

const PROVIDER_DEFAULTS: Record<string, string> = {
  openai: 'https://api.openai.com/v1',
  anthropic: 'https://api.anthropic.com/v1',
  openrouter: 'https://openrouter.ai/api/v1',
}

export default function AdminConfig() {
  const [config, setConfig] = useState<AdminConfig | null>(null)
  const [form, setForm] = useState<AdminConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [editing, setEditing] = useState<string | null>(null)

  useEffect(() => {
    getAdminConfig()
      .then((c) => {
        setConfig(c)
        setForm(JSON.parse(JSON.stringify(c)))
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
      setEditing(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const updatePool = (provider: keyof AdminConfig['key_pool'], field: 'api_key' | 'api_base', value: string) => {
    if (!form) return
    setForm({
      ...form,
      key_pool: {
        ...form.key_pool,
        [provider]: { ...form.key_pool[provider], [field]: value },
      },
    })
  }

  const statusBadge = (hasKey: boolean) =>
    hasKey ? (
      <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-emerald-100 text-emerald-800">
        已配置
      </span>
    ) : (
      <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-slate-100 text-slate-600">
        未配置
      </span>
    )

  if (loading) {
    return (
      <div className="max-w-5xl mx-auto">
        <div className="bg-white rounded-lg border border-slate-200 shadow-sm p-8">
          <div className="animate-pulse space-y-4">
            <div className="h-6 bg-slate-200 rounded w-1/4" />
            <div className="h-12 bg-slate-100 rounded" />
            <div className="h-12 bg-slate-100 rounded" />
            <div className="h-12 bg-slate-100 rounded" />
          </div>
        </div>
      </div>
    )
  }

  const updateDefaultModel = (v: string) => {
    if (!form) return
    setForm({ ...form, default_model: v })
  }

  return (
    <div className="max-w-5xl mx-auto">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-slate-800">渠道管理</h1>
        <p className="text-sm text-slate-500 mt-1">管理 LLM API 渠道，宠物实例将使用此处配置的密钥调用模型</p>
      </div>

      {/* 默认模型 */}
      <div className="mb-6 bg-white rounded-lg border border-slate-200 shadow-sm p-5">
        <h2 className="font-semibold text-slate-800 mb-2">默认模型</h2>
        <p className="text-sm text-slate-500 mb-3">宠物实例对话时使用的模型，如 gpt-4o、claude-3-5-sonnet、openrouter/auto 等</p>
        <input
          type="text"
          value={form?.default_model ?? ''}
          onChange={(e) => updateDefaultModel(e.target.value)}
          placeholder="gpt-4o"
          className="w-full max-w-md px-3 py-2 border border-slate-300 rounded-lg text-sm font-mono focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500"
        />
        <p className="text-xs text-slate-400 mt-1">当前使用：{form?.default_model || 'gpt-4o'}</p>
      </div>

      {error && (
        <div className="mb-4 p-4 rounded-lg bg-red-50 border border-red-100 text-red-700 text-sm">{error}</div>
      )}
      {success && (
        <div className="mb-4 p-4 rounded-lg bg-emerald-50 border border-emerald-100 text-emerald-700 text-sm">
          {success}
        </div>
      )}

      <div className="bg-white rounded-lg border border-slate-200 shadow-sm overflow-hidden">
        {form && (
          <form onSubmit={handleSave}>
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="bg-slate-50 border-b border-slate-200">
                    <th className="text-left py-3 px-4 text-sm font-medium text-slate-600">渠道</th>
                    <th className="text-left py-3 px-4 text-sm font-medium text-slate-600">状态</th>
                    <th className="text-left py-3 px-4 text-sm font-medium text-slate-600">API Key</th>
                    <th className="text-left py-3 px-4 text-sm font-medium text-slate-600">API Base</th>
                    <th className="text-left py-3 px-4 text-sm font-medium text-slate-600 w-20">操作</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {(['openai', 'anthropic', 'openrouter'] as const).map((provider) => (
                    <tr key={provider} className="hover:bg-slate-50/50">
                      <td className="py-3 px-4">
                        <span className="font-medium text-slate-800">{PROVIDER_LABELS[provider]}</span>
                      </td>
                      <td className="py-3 px-4">{statusBadge(!!config?.key_pool[provider].api_key)}</td>
                      <td className="py-3 px-4">
                        {editing === provider ? (
                          <input
                            type="password"
                            value={form.key_pool[provider].api_key}
                            onChange={(e) => updatePool(provider, 'api_key', e.target.value)}
                            placeholder="sk-..."
                            className="w-full max-w-xs px-3 py-2 border border-slate-300 rounded text-sm font-mono focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500"
                          />
                        ) : (
                          <span className="text-sm font-mono text-slate-600">
                            {config?.key_pool[provider].api_key || '—'}
                          </span>
                        )}
                      </td>
                      <td className="py-3 px-4">
                        {editing === provider ? (
                          <input
                            type="url"
                            value={form.key_pool[provider].api_base}
                            onChange={(e) => updatePool(provider, 'api_base', e.target.value)}
                            placeholder={PROVIDER_DEFAULTS[provider]}
                            className="w-full max-w-xs px-3 py-2 border border-slate-300 rounded text-sm focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500"
                          />
                        ) : (
                          <span className="text-sm text-slate-600 truncate max-w-[200px] block">
                            {form.key_pool[provider].api_base || PROVIDER_DEFAULTS[provider]}
                          </span>
                        )}
                      </td>
                      <td className="py-3 px-4">
                        {editing === provider ? (
                          <button
                            type="button"
                            onClick={() => setEditing(null)}
                            className="text-sm text-slate-600 hover:text-slate-800"
                          >
                            取消
                          </button>
                        ) : (
                          <button
                            type="button"
                            onClick={() => setEditing(provider)}
                            className="text-sm text-indigo-600 hover:text-indigo-700 font-medium"
                          >
                            编辑
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="px-4 py-4 bg-slate-50 border-t border-slate-200 flex justify-end gap-2">
              {(editing || form !== config) && (
                <button
                  type="button"
                  onClick={() => {
                    setForm(config ? JSON.parse(JSON.stringify(config)) : form)
                    setEditing(null)
                  }}
                  className="px-4 py-2 text-sm border border-slate-300 rounded-lg text-slate-700 hover:bg-slate-100"
                >
                  重置
                </button>
              )}
              <button
                type="submit"
                disabled={saving}
                className="px-4 py-2 text-sm bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 disabled:opacity-50"
              >
                {saving ? '保存中...' : '保存'}
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  )
}

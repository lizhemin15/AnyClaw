import { useState, useEffect } from 'react'
import { getAdminConfig, putAdminConfig, type AdminConfig } from '../api'

export default function AdminConfig() {
  const [config, setConfig] = useState<AdminConfig | null>(null)
  const [form, setForm] = useState<AdminConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

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
      setSuccess('保存成功，重启后生效')
      setConfig(form)
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

  if (loading) return <p className="text-slate-500 py-8">加载中...</p>

  return (
    <div className="max-w-2xl mx-auto">
      <h1 className="text-xl font-semibold text-slate-800 mb-2">AI 配置</h1>
      <p className="text-sm text-slate-500 mb-6">管理 LLM API Key，宠物实例将使用此处配置的密钥调用模型</p>

      {error && <p className="mb-4 text-sm text-red-600 bg-red-50 p-3 rounded-xl">{error}</p>}
      {success && <p className="mb-4 text-sm text-green-600 bg-green-50 p-3 rounded-xl">{success}</p>}

      {form && (
        <form onSubmit={handleSave} className="space-y-6">
          {(['openai', 'anthropic', 'openrouter'] as const).map((provider) => (
            <div key={provider} className="bg-white border border-slate-200 rounded-xl p-4">
              <h2 className="font-medium text-slate-800 mb-3 capitalize">{provider}</h2>
              <div className="space-y-3">
                <div>
                  <label className="block text-sm text-slate-600 mb-1">API Key</label>
                  <input
                    type="password"
                    value={form.key_pool[provider].api_key}
                    onChange={(e) => updatePool(provider, 'api_key', e.target.value)}
                    placeholder={config?.key_pool[provider].api_key ? '已配置，输入新值可覆盖' : 'sk-...'}
                    className="w-full px-4 py-2 border border-slate-300 rounded-lg font-mono text-sm"
                  />
                  {config?.key_pool[provider].api_key && (
                    <p className="text-xs text-slate-500 mt-1">当前: {config.key_pool[provider].api_key}</p>
                  )}
                </div>
                <div>
                  <label className="block text-sm text-slate-600 mb-1">API Base（可选）</label>
                  <input
                    type="url"
                    value={form.key_pool[provider].api_base}
                    onChange={(e) => updatePool(provider, 'api_base', e.target.value)}
                    placeholder={
                      provider === 'openai'
                        ? 'https://api.openai.com/v1'
                        : provider === 'anthropic'
                          ? 'https://api.anthropic.com/v1'
                          : 'https://openrouter.ai/api/v1'
                    }
                    className="w-full px-4 py-2 border border-slate-300 rounded-lg text-sm"
                  />
                </div>
              </div>
            </div>
          ))}
          <button
            type="submit"
            disabled={saving}
            className="w-full sm:w-auto px-6 py-3 bg-slate-800 text-white rounded-xl active:bg-slate-700 disabled:opacity-50 min-h-[48px]"
          >
            {saving ? '保存中...' : '保存'}
          </button>
        </form>
      )}
    </div>
  )
}

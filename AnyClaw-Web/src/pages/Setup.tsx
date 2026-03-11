import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { getSetupStatus, setupDatabase, setupAdmin } from '../api'

type Step = 'db' | 'admin' | 'done'

export default function Setup() {
  const [step, setStep] = useState<Step>('db')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [dbForm, setDbForm] = useState({ host: '', port: 3306, user: '', password: '', database: '' })
  const [adminForm, setAdminForm] = useState({ email: '', password: '' })
  const [submitting, setSubmitting] = useState(false)
  const navigate = useNavigate()

  useEffect(() => {
    getSetupStatus()
      .then((res) => {
        const { configured, needs_admin_only } = res as { configured?: boolean; needs_admin_only?: boolean }
        if (configured) {
          navigate('/login', { replace: true })
        } else {
          if (needs_admin_only) setStep('admin')
          setLoading(false)
        }
      })
      .catch(() => setLoading(false))
  }, [navigate])

  const handleDbSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSubmitting(true)
    setError('')
    try {
      await setupDatabase(dbForm)
      setStep('admin')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed')
    } finally {
      setSubmitting(false)
    }
  }

  const handleAdminSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSubmitting(true)
    setError('')
    try {
      await setupAdmin(adminForm)
      setStep('done')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed')
    } finally {
      setSubmitting(false)
    }
  }

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-slate-50">
        <div className="text-slate-500">加载中...</div>
      </div>
    )
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-slate-50 p-4 py-8">
      <div className="w-full max-w-md bg-white rounded-2xl shadow-lg p-6 sm:p-8">
        <h1 className="text-xl font-semibold text-slate-800 mb-2">OpenClaw 初始化</h1>
        <p className="text-sm text-slate-500 mb-6">首次使用请配置数据库和管理员账号</p>

        {error && (
          <p className="mb-4 text-sm text-red-600 bg-red-50 p-3 rounded-xl">{error}</p>
        )}

        {step === 'db' && (
          <form onSubmit={handleDbSubmit} className="space-y-5">
            <div>
              <label className="block text-sm font-medium text-slate-700 mb-2">MySQL 地址</label>
              <input
                value={dbForm.host}
                onChange={(e) => setDbForm((f) => ({ ...f, host: e.target.value }))}
                className="w-full px-4 py-3 border border-slate-300 rounded-xl"
                placeholder="localhost 或 IP"
                required
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-700 mb-2">端口</label>
              <input
                type="number"
                value={dbForm.port}
                onChange={(e) => setDbForm((f) => ({ ...f, port: parseInt(e.target.value) || 3306 }))}
                className="w-full px-4 py-3 border border-slate-300 rounded-xl"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-700 mb-2">用户名</label>
              <input
                value={dbForm.user}
                onChange={(e) => setDbForm((f) => ({ ...f, user: e.target.value }))}
                className="w-full px-4 py-3 border border-slate-300 rounded-xl"
                required
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-700 mb-2">密码</label>
              <input
                type="password"
                value={dbForm.password}
                onChange={(e) => setDbForm((f) => ({ ...f, password: e.target.value }))}
                className="w-full px-4 py-3 border border-slate-300 rounded-xl"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-700 mb-2">数据库名</label>
              <input
                value={dbForm.database}
                onChange={(e) => setDbForm((f) => ({ ...f, database: e.target.value }))}
                className="w-full px-4 py-3 border border-slate-300 rounded-xl"
                placeholder="openclaw"
                required
              />
            </div>
            <button
              type="submit"
              disabled={submitting}
              className="w-full py-3 bg-slate-800 text-white rounded-xl active:bg-slate-700 disabled:opacity-50 min-h-[48px] touch-target"
            >
              {submitting ? '连接中...' : '下一步'}
            </button>
          </form>
        )}

        {step === 'admin' && (
          <form onSubmit={handleAdminSubmit} className="space-y-5">
            <p className="text-sm text-green-600 bg-green-50 p-3 rounded-xl">数据库配置已保存</p>
            <div>
              <label className="block text-sm font-medium text-slate-700 mb-2">管理员邮箱</label>
              <input
                type="email"
                value={adminForm.email}
                onChange={(e) => setAdminForm((f) => ({ ...f, email: e.target.value }))}
                className="w-full px-4 py-3 border border-slate-300 rounded-xl"
                required
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-700 mb-2">密码（至少 6 位）</label>
              <input
                type="password"
                value={adminForm.password}
                onChange={(e) => setAdminForm((f) => ({ ...f, password: e.target.value }))}
                className="w-full px-4 py-3 border border-slate-300 rounded-xl"
                minLength={6}
                required
              />
            </div>
            <button
              type="submit"
              disabled={submitting}
              className="w-full py-3 bg-slate-800 text-white rounded-xl active:bg-slate-700 disabled:opacity-50 min-h-[48px] touch-target"
            >
              {submitting ? '创建中...' : '完成'}
            </button>
          </form>
        )}

        {step === 'done' && (
          <div className="space-y-4">
            <p className="text-green-600 font-medium">初始化完成！</p>
            <p className="text-sm text-slate-500 leading-relaxed">
              请重启应用使配置生效，然后访问首页登录。<br />
              Docker: docker restart &lt;容器名&gt;
            </p>
          </div>
        )}
      </div>
    </div>
  )
}

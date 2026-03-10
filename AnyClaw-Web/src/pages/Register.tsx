import { useState, useEffect } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { register, sendVerificationCode, getAuthConfig, setToken, type User } from '../api'

interface RegisterProps {
  onRegister: (user: User) => void;
}

export default function Register({ onRegister }: RegisterProps) {
  const [email, setEmail] = useState('')
  const [code, setCode] = useState('')
  const [password, setPassword] = useState('')
  const [inviteCode, setInviteCode] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [sendCodeLoading, setSendCodeLoading] = useState(false)
  const [sendCodeCooldown, setSendCodeCooldown] = useState(0)
  const [verificationRequired, setVerificationRequired] = useState<boolean | null>(null)
  const [step, setStep] = useState(1)
  const [searchParams] = useSearchParams()

  useEffect(() => {
    const invite = searchParams.get('invite') || searchParams.get('ref')
    if (invite) setInviteCode(invite)
  }, [searchParams])

  useEffect(() => {
    getAuthConfig()
      .then((c) => setVerificationRequired(c.email_verification_required))
      .catch(() => setVerificationRequired(false))
  }, [])

  useEffect(() => {
    if (sendCodeCooldown <= 0) return
    const t = setInterval(() => setSendCodeCooldown((s) => (s <= 1 ? 0 : s - 1)), 1000)
    return () => clearInterval(t)
  }, [sendCodeCooldown])

  const handleSendCode = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!email.trim()) return
    setError('')
    setSendCodeLoading(true)
    try {
      await sendVerificationCode(email.trim())
      setStep(2)
      setSendCodeCooldown(60)
    } catch (err) {
      setError(err instanceof Error ? err.message : '发送失败')
    } finally {
      setSendCodeLoading(false)
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const res = await register(email.trim(), password, {
        code: verificationRequired ? code : undefined,
        inviteCode: inviteCode.trim() || undefined,
      })
      setToken(res.access_token)
      onRegister(res.user)
    } catch (err) {
      setError(err instanceof Error ? err.message : '注册失败')
    } finally {
      setLoading(false)
    }
  }

  if (verificationRequired === null) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-slate-50">
        <div className="text-slate-500">加载中...</div>
      </div>
    )
  }

  const showVerificationFlow = verificationRequired === true

  return (
    <div className="min-h-screen flex items-center justify-center bg-slate-50 px-4 py-8">
      <div className="w-full max-w-sm bg-white rounded-2xl shadow-sm border border-slate-200 p-6 sm:p-8">
        <h1 className="text-xl font-semibold text-slate-800 mb-6">注册</h1>

        {showVerificationFlow && step === 1 ? (
          <form onSubmit={handleSendCode} className="space-y-5">
            <div>
              <label htmlFor="email" className="block text-sm font-medium text-slate-700 mb-2">
                邮箱
              </label>
              <input
                id="email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                autoComplete="email"
                className="w-full px-4 py-3 border border-slate-300 rounded-xl focus:outline-none focus:ring-2 focus:ring-slate-400 focus:border-transparent"
                placeholder="you@example.com"
              />
            </div>
            {error && (
              <p className="text-sm text-red-600 bg-red-50 p-3 rounded-lg">{error}</p>
            )}
            <button
              type="submit"
              disabled={sendCodeLoading || sendCodeCooldown > 0}
              className="w-full py-3 px-4 bg-slate-800 text-white rounded-xl active:bg-slate-700 disabled:opacity-50 min-h-[48px] touch-target"
            >
              {sendCodeCooldown > 0
                ? `${sendCodeCooldown} 秒后重试`
                : sendCodeLoading
                  ? '发送中...'
                  : '发送验证码'}
            </button>
          </form>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-5">
            <div>
              <label htmlFor="email" className="block text-sm font-medium text-slate-700 mb-2">
                邮箱
              </label>
              <input
                id="email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                autoComplete="email"
                disabled={showVerificationFlow}
                className="w-full px-4 py-3 border border-slate-300 rounded-xl focus:outline-none focus:ring-2 focus:ring-slate-400 focus:border-transparent disabled:bg-slate-50 disabled:text-slate-600"
                placeholder="you@example.com"
              />
            </div>
            {showVerificationFlow && (
              <div>
                <label htmlFor="code" className="block text-sm font-medium text-slate-700 mb-2">
                  验证码
                </label>
                <input
                  id="code"
                  type="text"
                  value={code}
                  onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                  required
                  maxLength={6}
                  placeholder="请输入 6 位验证码"
                  className="w-full px-4 py-3 border border-slate-300 rounded-xl focus:outline-none focus:ring-2 focus:ring-slate-400 font-mono text-lg tracking-widest"
                />
              </div>
            )}
            <div>
              <label htmlFor="invite" className="block text-sm font-medium text-slate-700 mb-2">
                邀请码（可选，注册得奖励，受邀充值你可获返利）
              </label>
              <input
                id="invite"
                type="text"
                value={inviteCode}
                onChange={(e) => setInviteCode(e.target.value)}
                className="w-full px-4 py-3 border border-slate-300 rounded-xl"
                placeholder="邀请码"
              />
            </div>
            <div>
              <label htmlFor="password" className="block text-sm font-medium text-slate-700 mb-2">
                密码（至少 6 位）
              </label>
              <input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                minLength={6}
                autoComplete="new-password"
                className="w-full px-4 py-3 border border-slate-300 rounded-xl focus:outline-none focus:ring-2 focus:ring-slate-400 focus:border-transparent"
              />
            </div>
            {error && (
              <p className="text-sm text-red-600 bg-red-50 p-3 rounded-lg">{error}</p>
            )}
            <button
              type="submit"
              disabled={loading}
              className="w-full py-3 px-4 bg-slate-800 text-white rounded-xl active:bg-slate-700 disabled:opacity-50 min-h-[48px] touch-target"
            >
              {loading ? '注册中...' : '注册'}
            </button>
          </form>
        )}

        <p className="mt-5 text-sm text-slate-600 text-center">
          已有账号？{' '}
          <Link to="/login" className="text-slate-800 font-medium active:underline">
            登录
          </Link>
        </p>
      </div>
    </div>
  )
}

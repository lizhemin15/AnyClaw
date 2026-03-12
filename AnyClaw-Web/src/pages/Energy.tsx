import { useState, useEffect } from 'react'
import {
  getAdminUsers,
  adminRechargeUser,
  adminCreateUser,
  adminUpdateUser,
  getMe,
  adminGenerateActivationCodes,
  adminListActivationCodes,
  adminVerifyActivationCode,
  type User,
  type UserWithInstances,
  type ActivationCode,
} from '../api'

export default function Energy() {
  const [users, setUsers] = useState<UserWithInstances[]>([])
  const [me, setMe] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [recharging, setRecharging] = useState<number | null>(null)
  const [amount, setAmount] = useState<Record<number, string>>({})
  const [tab, setTab] = useState<'users' | 'codes'>('users')
  const [codes, setCodes] = useState<ActivationCode[]>([])
  const [codesLoading, setCodesLoading] = useState(false)
  const [genCount, setGenCount] = useState('10')
  const [genEnergy, setGenEnergy] = useState('500')
  const [genMemo, setGenMemo] = useState('')
  const [generating, setGenerating] = useState(false)
  const [newCodes, setNewCodes] = useState<string[]>([])
  const [verifyCode, setVerifyCode] = useState('')
  const [verifyResult, setVerifyResult] = useState<{ valid: boolean; energy?: number; message?: string } | null>(null)
  const [showAddUser, setShowAddUser] = useState(false)
  const [addEmail, setAddEmail] = useState('')
  const [addPassword, setAddPassword] = useState('')
  const [addRole, setAddRole] = useState<'user' | 'admin'>('user')
  const [addEnergy, setAddEnergy] = useState('0')
  const [adding, setAdding] = useState(false)
  const [editing, setEditing] = useState<number | null>(null)
  const [editRole, setEditRole] = useState<'user' | 'admin'>('user')
  const [editEnergy, setEditEnergy] = useState('')

  const load = () => {
    setLoading(true)
    Promise.all([getAdminUsers(), getMe()])
      .then(([u, m]) => {
        setUsers(u)
        setMe(m)
      })
      .catch((err) => setError(err instanceof Error ? err.message : '加载失败'))
      .finally(() => setLoading(false))
  }

  const loadCodes = (status: 'unused' | 'used' | 'all' = 'all') => {
    setCodesLoading(true)
    adminListActivationCodes(status, 100, 0)
      .then((r) => setCodes(r.items || []))
      .catch(() => setCodes([]))
      .finally(() => setCodesLoading(false))
  }

  useEffect(() => {
    load()
  }, [])

  useEffect(() => {
    if (tab === 'codes') loadCodes('all')
  }, [tab])

  const handleRecharge = async (u: UserWithInstances) => {
    const val = amount[u.id]?.trim()
    const amt = parseInt(val ?? '', 10)
    if (!val || isNaN(amt) || amt <= 0) {
      setError('请输入有效金币数量')
      return
    }
    setRecharging(u.id)
    setError('')
    try {
      await adminRechargeUser(u.id, amt)
      setAmount((prev) => ({ ...prev, [u.id]: '' }))
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : '充值失败')
    } finally {
      setRecharging(null)
    }
  }

  const handleGenerate = async () => {
    const c = parseInt(genCount, 10)
    const e = parseInt(genEnergy, 10)
    if (!c || c < 1 || c > 100 || !e || e < 1) {
      setError('数量 1-100，金币数需为正整数')
      return
    }
    setGenerating(true)
    setError('')
    setNewCodes([])
    try {
      const res = await adminGenerateActivationCodes(c, e, genMemo.trim() || undefined)
      setNewCodes(res.codes || [])
      loadCodes()
    } catch (err) {
      setError(err instanceof Error ? err.message : '生成失败')
    } finally {
      setGenerating(false)
    }
  }

  const handleVerify = async () => {
    if (!verifyCode.trim()) return
    setVerifyResult(null)
    try {
      const res = await adminVerifyActivationCode(verifyCode.trim().toUpperCase())
      setVerifyResult(res)
    } catch {
      setVerifyResult({ valid: false, message: '验证失败' })
    }
  }

  const handleAddUser = async () => {
    const email = addEmail.trim().toLowerCase()
    if (!email || addPassword.length < 6) {
      setError('邮箱必填，密码至少 6 位')
      return
    }
    const energy = parseInt(addEnergy, 10) || 0
    setAdding(true)
    setError('')
    try {
      await adminCreateUser(email, addPassword, addRole, energy)
      setShowAddUser(false)
      setAddEmail('')
      setAddPassword('')
      setAddRole('user')
      setAddEnergy('0')
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : '添加失败')
    } finally {
      setAdding(false)
    }
  }

  const handleUpdateUser = async (u: UserWithInstances) => {
    const role = editRole
    const energy = parseInt(editEnergy, 10)
    if (isNaN(energy) || energy < 0) {
      setError('金币需为非负整数')
      return
    }
    setError('')
    try {
      await adminUpdateUser(u.id, { role, energy })
      setEditing(null)
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : '更新失败')
    }
  }

  const openEdit = (u: UserWithInstances) => {
    setEditing(u.id)
    setEditRole((u.role === 'admin' ? 'admin' : 'user') as 'user' | 'admin')
    setEditEnergy(String(u.energy))
  }

  return (
    <div className="max-w-4xl mx-auto">
      <div className="flex gap-2 mb-4">
        <button
          type="button"
          onClick={() => setTab('users')}
          className={`px-4 py-2 text-sm rounded-lg ${tab === 'users' ? 'bg-slate-800 text-white' : 'bg-slate-100 text-slate-600'}`}
        >
          用户管理
        </button>
        <button
          type="button"
          onClick={() => setTab('codes')}
          className={`px-4 py-2 text-sm rounded-lg ${tab === 'codes' ? 'bg-slate-800 text-white' : 'bg-slate-100 text-slate-600'}`}
        >
          激活码
        </button>
      </div>

      <h1 className="text-xl font-semibold text-slate-800 mb-4">{tab === 'users' ? '用户管理' : '激活码管理'}</h1>
      <p className="text-sm text-slate-500 mb-4">
        {tab === 'users' ? '添加用户、设置权限（普通用户/管理员）、充值金币' : '生成激活码供用户兑换，支持外部平台自动发货核销。API: POST /admin/activation-codes（生成）、POST /admin/activation-codes/verify（核销前校验）、POST /admin/activation-codes/redeem（代用户兑换）'}
      </p>

      {error && (
        <p className="mb-4 text-sm text-red-600 bg-red-50 p-3 rounded-xl">{error}</p>
      )}

      {tab === 'codes' && (
        <div className="mb-6 p-4 bg-white border border-slate-200 rounded-xl space-y-4">
          <h2 className="font-medium text-slate-800">生成激活码</h2>
          <div className="flex flex-wrap gap-3 items-end">
            <div>
              <label className="block text-xs text-slate-500 mb-1">数量</label>
              <input type="number" min={1} max={100} value={genCount} onChange={(e) => setGenCount(e.target.value)} className="w-20 px-3 py-2 border rounded-lg text-sm" />
            </div>
            <div>
              <label className="block text-xs text-slate-500 mb-1">金币/个</label>
              <input type="number" min={1} value={genEnergy} onChange={(e) => setGenEnergy(e.target.value)} className="w-24 px-3 py-2 border rounded-lg text-sm" />
            </div>
            <div>
              <label className="block text-xs text-slate-500 mb-1">备注</label>
              <input type="text" value={genMemo} onChange={(e) => setGenMemo(e.target.value)} placeholder="可选" className="w-32 px-3 py-2 border rounded-lg text-sm" />
            </div>
            <button onClick={handleGenerate} disabled={generating} className="px-4 py-2 bg-indigo-600 text-white rounded-lg text-sm disabled:opacity-50">
              {generating ? '生成中...' : '生成'}
            </button>
          </div>
          {newCodes.length > 0 && (
            <div className="mt-4 p-3 bg-slate-50 rounded-lg">
              <p className="text-sm font-medium text-slate-700 mb-2">已生成 {newCodes.length} 个，请妥善保存：</p>
              <textarea readOnly value={newCodes.join('\n')} rows={6} className="w-full px-3 py-2 border rounded-lg text-sm font-mono" />
            </div>
          )}
          <div className="pt-4 border-t">
            <h3 className="text-sm font-medium text-slate-700 mb-2">核销前校验（API 对接用）</h3>
            <div className="flex gap-2">
              <input
                type="text"
                value={verifyCode}
                onChange={(e) => { setVerifyCode(e.target.value.toUpperCase()); setVerifyResult(null) }}
                placeholder="输入激活码"
                className="flex-1 px-3 py-2 border rounded-lg text-sm font-mono uppercase"
              />
              <button onClick={handleVerify} className="px-4 py-2 bg-slate-100 text-slate-700 rounded-lg text-sm">校验</button>
            </div>
            {verifyResult && (
              <p className={`mt-2 text-sm ${verifyResult.valid ? 'text-emerald-600' : 'text-red-600'}`}>
                {verifyResult.valid ? `有效，可兑换 ${verifyResult.energy} 金币` : (verifyResult.message || '无效')}
              </p>
            )}
          </div>
        </div>
      )}

      {tab === 'codes' && (
        <div className="mb-6">
          <div className="flex gap-2 mb-2">
            {(['all', 'unused', 'used'] as const).map((s) => (
              <button key={s} onClick={() => loadCodes(s)} className="px-3 py-1.5 text-sm rounded-lg bg-slate-100 text-slate-600 hover:bg-slate-200">
                {s === 'all' ? '全部' : s === 'unused' ? '未使用' : '已使用'}
              </button>
            ))}
          </div>
          {codesLoading ? (
            <p className="text-slate-500 py-4">加载中...</p>
          ) : (
            <div className="space-y-2 max-h-64 overflow-y-auto">
              {codes.map((c) => (
                <div key={c.code} className="flex items-center justify-between py-2 px-3 bg-white border rounded-lg text-sm">
                  <span className="font-mono">{c.code}</span>
                  <span>{c.energy} 金币</span>
                  {c.used_by ? <span className="text-slate-500">已用</span> : <span className="text-emerald-600">未用</span>}
                </div>
              ))}
              {codes.length === 0 && <p className="text-slate-500 py-4">暂无激活码</p>}
            </div>
          )}
        </div>
      )}

      {tab === 'users' && loading ? (
        <p className="text-slate-500 py-8">加载中...</p>
      ) : tab === 'users' ? (
        <div className="space-y-3">
          <div className="flex justify-between items-center">
            <h2 className="font-medium text-slate-800">用户列表</h2>
            <button
              type="button"
              onClick={() => { setShowAddUser(true); setError('') }}
              className="px-4 py-2 text-sm bg-indigo-600 text-white rounded-lg hover:bg-indigo-700"
            >
              添加用户
            </button>
          </div>
          {showAddUser && (
            <div className="bg-slate-50 border border-slate-200 rounded-xl p-4 space-y-3">
              <h3 className="font-medium text-slate-700">新建用户</h3>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <input type="email" value={addEmail} onChange={(e) => setAddEmail(e.target.value)} placeholder="邮箱" className="px-3 py-2 border rounded-lg text-sm" />
                <input type="password" value={addPassword} onChange={(e) => setAddPassword(e.target.value)} placeholder="密码（至少6位）" className="px-3 py-2 border rounded-lg text-sm" />
                <select value={addRole} onChange={(e) => setAddRole(e.target.value as 'user' | 'admin')} className="px-3 py-2 border rounded-lg text-sm">
                  <option value="user">普通用户</option>
                  <option value="admin">管理员</option>
                </select>
                <input type="number" min={0} value={addEnergy} onChange={(e) => setAddEnergy(e.target.value)} placeholder="初始金币" className="px-3 py-2 border rounded-lg text-sm" />
              </div>
              <div className="flex gap-2">
                <button onClick={handleAddUser} disabled={adding} className="px-4 py-2 bg-indigo-600 text-white rounded-lg text-sm disabled:opacity-50">{adding ? '添加中...' : '添加'}</button>
                <button onClick={() => setShowAddUser(false)} className="px-4 py-2 bg-slate-200 text-slate-700 rounded-lg text-sm">取消</button>
              </div>
            </div>
          )}
          {users.map((u) => (
            <div
              key={u.id}
              className="bg-white border border-slate-200 rounded-xl p-4 flex flex-col gap-3"
            >
              {editing === u.id ? (
                <>
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium text-slate-800">{u.email}</span>
                    {u.id === me?.id && <span className="px-2 py-0.5 text-xs bg-amber-100 text-amber-800 rounded">当前</span>}
                  </div>
                  <div className="flex flex-wrap gap-3 items-center">
                    <div>
                      <label className="block text-xs text-slate-500 mb-0.5">权限</label>
                      <select value={editRole} onChange={(e) => setEditRole(e.target.value as 'user' | 'admin')} className="px-3 py-2 border rounded-lg text-sm">
                        <option value="user">普通用户</option>
                        <option value="admin">管理员</option>
                      </select>
                    </div>
                    <div>
                      <label className="block text-xs text-slate-500 mb-0.5">金币</label>
                      <input type="number" min={0} value={editEnergy} onChange={(e) => setEditEnergy(e.target.value)} className="w-24 px-3 py-2 border rounded-lg text-sm" />
                    </div>
                    <div className="flex gap-2 pt-5">
                      <button onClick={() => handleUpdateUser(u)} className="px-4 py-2 text-sm bg-slate-800 text-white rounded-lg">保存</button>
                      <button onClick={() => setEditing(null)} className="px-4 py-2 text-sm bg-slate-200 text-slate-700 rounded-lg">取消</button>
                    </div>
                  </div>
                </>
              ) : (
                <>
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium text-slate-800">{u.email}</span>
                    {u.id === me?.id && <span className="px-2 py-0.5 text-xs bg-amber-100 text-amber-800 rounded">当前</span>}
                    <span className="px-2 py-0.5 text-xs bg-slate-100 text-slate-600 rounded">{u.role === 'admin' ? '管理员' : '普通用户'}</span>
                  </div>
                  <p className="text-sm text-slate-500">ID: {u.id} · 金币: {u.energy} · 实例: {u.instance_count ?? 0}</p>
                  <div className="flex flex-wrap gap-2 items-center">
                    <button onClick={() => openEdit(u)} className="px-3 py-1.5 text-sm bg-slate-100 text-slate-700 rounded-lg hover:bg-slate-200">编辑</button>
                    <input
                      type="number"
                      min={1}
                      placeholder="充值数量"
                      value={amount[u.id] ?? ''}
                      onChange={(e) => setAmount((prev) => ({ ...prev, [u.id]: e.target.value }))}
                      className="w-24 px-3 py-2 border border-slate-300 rounded-lg text-sm"
                    />
                    <button
                      onClick={() => handleRecharge(u)}
                      disabled={!!recharging}
                      className="px-4 py-2 text-sm bg-slate-800 text-white rounded-lg active:bg-slate-700 disabled:opacity-50"
                    >
                      {recharging === u.id ? '充值中...' : '充值'}
                    </button>
                  </div>
                </>
              )}
            </div>
          ))}
        </div>
      ) : null}
    </div>
  )
}

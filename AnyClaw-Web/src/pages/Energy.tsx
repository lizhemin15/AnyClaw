import { useState, useEffect } from 'react'
import {
  getAdminUsers,
  adminRechargeUser,
  adminCreateUser,
  adminUpdateUser,
  getMe,
  type User,
  type UserWithInstances,
} from '../api'

export default function Energy() {
  const [users, setUsers] = useState<UserWithInstances[]>([])
  const [me, setMe] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [recharging, setRecharging] = useState<number | null>(null)
  const [amount, setAmount] = useState<Record<number, string>>({})
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

  useEffect(() => {
    load()
  }, [])

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
      <h1 className="text-xl font-semibold text-slate-800 mb-4">用户管理</h1>
      <p className="text-sm text-slate-500 mb-4">
        添加用户、设置权限（普通用户/管理员）、充值金币。用户扫码付款并备注邮箱后，在此根据付款记录人工审核并充值。
      </p>

      {error && (
        <p className="mb-4 text-sm text-red-600 bg-red-50 p-3 rounded-xl">{error}</p>
      )}

      {loading ? (
        <p className="text-slate-500 py-8">加载中...</p>
      ) : (
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
      )}
    </div>
  )
}

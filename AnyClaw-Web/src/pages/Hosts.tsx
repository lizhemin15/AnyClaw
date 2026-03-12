import { useState, useEffect, useCallback } from 'react'
import { Link } from 'react-router-dom'
import {
  getHosts,
  createHost,
  updateHost,
  deleteHost,
  checkHostStatus,
  getHostInstanceImageStatus,
  pullAndRestartInstances,
  drainHost,
  getAdminInstances,
  adminDeleteInstance,
  adminMigrateInstance,
  type Host,
  type CreateHostRequest,
  type AdminInstance,
} from '../api'

export default function Hosts() {
  const [hosts, setHosts] = useState<Host[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [modal, setModal] = useState<'add' | 'edit' | null>(null)
  const [editing, setEditing] = useState<Host | null>(null)
  const [form, setForm] = useState<CreateHostRequest & { id?: string }>({
    name: '',
    addr: '',
    ssh_port: 22,
    ssh_user: '',
    ssh_key: '',
    ssh_password: '',
    docker_image: '',
    enabled: true,
    instance_capacity: 0,
  })
  const [submitting, setSubmitting] = useState(false)
  const [checking, setChecking] = useState<string | null>(null)
  const [instanceImageStatus, setInstanceImageStatus] = useState<Record<string, { update_available: boolean; image: string; instance_count: number; message?: string }>>({})
  const [pullingInstances, setPullingInstances] = useState<string | null>(null)
  const [draining, setDraining] = useState<string | null>(null)
  const [instances, setInstances] = useState<AdminInstance[]>([])
  const [instancesLoading, setInstancesLoading] = useState(true)
  const [deletingInst, setDeletingInst] = useState<number | null>(null)
  const [migratingInst, setMigratingInst] = useState<number | null>(null)
  const [migrateModal, setMigrateModal] = useState<AdminInstance | null>(null)

  const loadHosts = () => {
    setLoading(true)
    getHosts()
      .then(setHosts)
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load'))
      .finally(() => setLoading(false))
  }

  const loadInstances = () => {
    setInstancesLoading(true)
    getAdminInstances()
      .then(setInstances)
      .catch(() => setInstances([]))
      .finally(() => setInstancesLoading(false))
  }

  useEffect(() => {
    loadHosts()
    loadInstances()
  }, [])

  const checkInstanceImageStatus = useCallback(async (id: string) => {
    try {
      const res = await getHostInstanceImageStatus(id)
      setInstanceImageStatus((prev) => ({
        ...prev,
        [id]: {
          update_available: res.update_available,
          image: res.image,
          instance_count: res.instance_count,
          message: res.message,
        },
      }))
    } catch {
      setInstanceImageStatus((prev) => ({ ...prev, [id]: { update_available: false, image: '', instance_count: 0 } }))
    }
  }, [])

  useEffect(() => {
    hosts.filter((h) => h.enabled).forEach((h) => {
      if (instanceImageStatus[h.id] === undefined) {
        checkInstanceImageStatus(h.id)
      }
    })
  }, [hosts, instanceImageStatus, checkInstanceImageStatus])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (submitting) return
    if (!form.name.trim() || !form.addr.trim() || !form.ssh_user.trim()) {
      setError('name, addr, ssh_user required')
      return
    }
    if (modal === 'add') {
      const hasKey = !!form.ssh_key?.trim()
      const hasPass = !!form.ssh_password?.trim()
      if (!hasKey && !hasPass) {
        setError('请填写 SSH 密码或私钥')
        return
      }
    }
    setSubmitting(true)
    setError('')
    try {
      if (modal === 'add') {
        await createHost(form)
      } else if (editing) {
        await updateHost(editing.id, form)
      }
      setModal(null)
      setEditing(null)
      setForm({ name: '', addr: '', ssh_port: 22, ssh_user: '', ssh_key: '', ssh_password: '', docker_image: '', enabled: true, instance_capacity: 0 })
      loadHosts()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save')
    } finally {
      setSubmitting(false)
    }
  }

  const handleCheck = async (id: string) => {
    setChecking(id)
    try {
      const { status } = await checkHostStatus(id)
      setHosts((prev) => prev.map((h) => (h.id === id ? { ...h, status } : h)))
      checkInstanceImageStatus(id)
    } catch {
      setHosts((prev) => prev.map((h) => (h.id === id ? { ...h, status: 'error' } : h)))
    } finally {
      setChecking(null)
    }
  }

  const handleDrain = async (h: Host) => {
    const count = instanceImageStatus[h.id]?.instance_count ?? 0
    if (count === 0) {
      alert('该主机无运行中实例')
      return
    }
    if (!confirm(`确定排空「${h.name}」？将把 ${count} 个实例迁移到其他主机。`)) return
    setDraining(h.id)
    setError('')
    try {
      const res = await drainHost(h.id)
      if (res.ok) {
        alert(res.message)
        checkInstanceImageStatus(h.id)
        loadInstances()
        loadHosts()
      } else {
        setError(res.message || '排空失败')
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : '排空失败')
    } finally {
      setDraining(null)
    }
  }

  const handlePullAndRestartInstances = async (h: Host) => {
    const imgStatus = instanceImageStatus[h.id]
    const count = imgStatus?.instance_count ?? 0
    const msg = count > 0
      ? `确定在「${h.name}」上拉取最新实例镜像并重启 ${count} 个实例？`
      : `确定在「${h.name}」上拉取最新实例镜像？（该主机暂无运行中的实例）`
    if (!confirm(msg)) return
    setPullingInstances(h.id)
    setError('')
    try {
      const res = await pullAndRestartInstances(h.id)
      if (res.ok) {
        setError('')
        alert(res.message)
        checkInstanceImageStatus(h.id)
        loadInstances()
      } else {
        setError(res.failed_ids?.length ? `${res.message}，失败 ID: ${res.failed_ids.join(', ')}` : res.message)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : '更新失败')
    } finally {
      setPullingInstances(null)
    }
  }

  const handleDelete = async (h: Host) => {
    if (!confirm(`Delete host "${h.name}"?`)) return
    try {
      await deleteHost(h.id)
      loadHosts()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete')
    }
  }

  const handleDeleteInstance = async (inst: AdminInstance) => {
    if (!confirm(`删除实例 #${inst.id}「${inst.name}」？将停止容器并删除数据。`)) return
    setDeletingInst(inst.id)
    try {
      await adminDeleteInstance(inst.id)
      loadInstances()
    } catch (err) {
      setError(err instanceof Error ? err.message : '删除失败')
    } finally {
      setDeletingInst(null)
    }
  }

  const handleMigrateInstance = async (inst: AdminInstance, targetHostId: string) => {
    if (!targetHostId || targetHostId === inst.host_id) return
    setMigratingInst(inst.id)
    setError('')
    try {
      const res = await adminMigrateInstance(inst.id, targetHostId)
      if (res.ok) {
        setMigrateModal(null)
        loadInstances()
        loadHosts()
      } else {
        setError(res.message || '迁移失败')
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : '迁移失败')
    } finally {
      setMigratingInst(null)
    }
  }

  const openEdit = (h: Host) => {
    setEditing(h)
    setForm({
      name: h.name,
      addr: h.addr,
      ssh_port: h.ssh_port,
      ssh_user: h.ssh_user,
      ssh_key: '',
      ssh_password: '',
      docker_image: h.docker_image || '',
      enabled: h.enabled,
      instance_capacity: h.instance_capacity ?? 0,
    })
    setModal('edit')
  }

  const statusColor = (s: string) =>
    s === 'online' ? 'bg-green-100 text-green-800' : s === 'error' ? 'bg-red-100 text-red-800' : 'bg-slate-100 text-slate-700'

  return (
    <div className="max-w-4xl mx-auto">
      <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-4 mb-4">
        <h1 className="text-xl font-semibold text-slate-800">服务器</h1>
        <button
          onClick={() => {
            setModal('add')
            setEditing(null)
            setForm({ name: '', addr: '', ssh_port: 22, ssh_user: '', ssh_key: '', ssh_password: '', docker_image: '', enabled: true, instance_capacity: 0 })
          }}
          className="w-full sm:w-auto px-6 py-3 bg-slate-800 text-white rounded-xl active:bg-slate-700 min-h-[48px] touch-target"
        >
          添加服务器
        </button>
      </div>

      {error && (
        <p className="mb-4 text-sm text-red-600 bg-red-50 p-3 rounded-xl">{error}</p>
      )}

      {loading ? (
        <p className="text-slate-500 py-8">加载中...</p>
      ) : hosts.length === 0 ? (
        <p className="text-slate-500 py-8">暂无服务器，点击上方添加</p>
      ) : (
        <div className="space-y-3">
          {hosts.map((h) => (
            <div
              key={h.id}
              className="bg-white border border-slate-200 rounded-xl p-4 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3"
            >
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium text-slate-800">{h.name}</span>
                  <span className={`px-2.5 py-1 text-xs rounded-full ${statusColor(h.status)}`}>{h.status === 'online' ? '在线' : h.status === 'error' ? '异常' : h.status}</span>
                  {!h.enabled && <span className="text-xs text-slate-500">已禁用</span>}
                </div>
                <p className="text-sm text-slate-500 mt-1 truncate">
                  {h.ssh_user}@{h.addr}:{h.ssh_port}
                  {h.docker_image && ` · ${h.docker_image}`}
                  {(instanceImageStatus[h.id]?.instance_count ?? 0) > 0 && (
                    <span className="ml-1 text-emerald-600">· {(instanceImageStatus[h.id]?.instance_count ?? 0)} 实例</span>
                  )}
                </p>
              </div>
              <div className="flex gap-2 flex-shrink-0 flex-wrap">
                <button
                  onClick={() => handleCheck(h.id)}
                  disabled={!!checking}
                  className="flex-1 sm:flex-none px-4 py-2 text-sm border border-slate-300 rounded-lg active:bg-slate-50 disabled:opacity-50 min-h-[44px]"
                >
                  {checking === h.id ? '检测中...' : '检测'}
                </button>
                <button
                  onClick={() => handleDrain(h)}
                  disabled={!!draining || !!pullingInstances || (instanceImageStatus[h.id]?.instance_count ?? 0) === 0}
                  title="将该主机上所有实例迁移到其他主机"
                  className="flex-1 sm:flex-none px-4 py-2 text-sm border border-amber-300 text-amber-700 rounded-lg active:bg-amber-50 disabled:opacity-50 min-h-[44px]"
                >
                  {draining === h.id ? '排空中...' : '排空'}
                </button>
                <button
                  onClick={() => handlePullAndRestartInstances(h)}
                  disabled={
                    !!pullingInstances ||
                    (instanceImageStatus[h.id] && !instanceImageStatus[h.id].update_available)
                  }
                  title={
                    instanceImageStatus[h.id]?.message ||
                    (instanceImageStatus[h.id] && !instanceImageStatus[h.id].update_available
                      ? `实例镜像已是最新 (${instanceImageStatus[h.id]?.image || ''})`
                      : `拉取 ${instanceImageStatus[h.id]?.image || ''} 并重启该主机上 ${instanceImageStatus[h.id]?.instance_count ?? 0} 个实例`)
                  }
                  className="flex-1 sm:flex-none px-4 py-2 text-sm bg-emerald-600 text-white rounded-lg active:bg-emerald-700 disabled:opacity-50 disabled:cursor-not-allowed min-h-[44px]"
                >
                  {pullingInstances === h.id
                    ? '拉取中...'
                    : instanceImageStatus[h.id] === undefined
                      ? '检查中...'
                      : instanceImageStatus[h.id].update_available
                        ? `更新实例 (${instanceImageStatus[h.id].instance_count})`
                        : '实例已最新'}
                </button>
                <button
                  onClick={() => openEdit(h)}
                  className="flex-1 sm:flex-none px-4 py-2 text-sm border border-slate-300 rounded-lg active:bg-slate-50 min-h-[44px]"
                >
                  编辑
                </button>
                <button
                  onClick={() => handleDelete(h)}
                  className="px-4 py-2 text-sm border border-red-200 text-red-600 rounded-lg active:bg-red-50 min-h-[44px]"
                >
                  删除
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* 实例列表 */}
      <div className="mt-10">
        <h2 className="text-lg font-semibold text-slate-800 mb-3">实例列表（AnyClaw 容器）</h2>
        {instancesLoading ? (
          <p className="text-slate-500 py-6">加载中...</p>
        ) : instances.length === 0 ? (
          <p className="text-slate-500 py-6">暂无实例</p>
        ) : (
          <div className="space-y-2">
            {instances.map((inst) => (
              <div
                key={inst.id}
                className="bg-white border border-slate-200 rounded-xl p-4 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium text-slate-800">#{inst.id} {inst.name}</span>
                    <span className={`px-2.5 py-1 text-xs rounded-full ${inst.status === 'running' ? 'bg-green-100 text-green-800' : inst.status === 'creating' ? 'bg-amber-100 text-amber-800' : 'bg-slate-100 text-slate-700'}`}>
                      {inst.status === 'running' ? '运行中' : inst.status === 'creating' ? '创建中' : inst.status}
                    </span>
                  </div>
                  <p className="text-sm text-slate-500 mt-1">
                    用户: {inst.user_email || '—'} · 宿主机: {inst.host_name || '—'}
                  </p>
                </div>
                <div className="flex gap-2 flex-shrink-0">
                  <Link
                    to={`/instances/${inst.id}`}
                    className="px-4 py-2 text-sm border border-slate-300 rounded-lg active:bg-slate-50 min-h-[44px] inline-block"
                  >
                    打开
                  </Link>
                  {inst.status === 'running' && inst.host_id && hosts.filter((h) => h.enabled && h.id !== inst.host_id).length > 0 && (
                    <button
                      onClick={() => setMigrateModal(inst)}
                      disabled={!!migratingInst}
                      className="px-4 py-2 text-sm border border-slate-300 rounded-lg active:bg-slate-50 disabled:opacity-50 min-h-[44px]"
                    >
                      迁移
                    </button>
                  )}
                  <button
                    onClick={() => handleDeleteInstance(inst)}
                    disabled={deletingInst === inst.id}
                    className="px-4 py-2 text-sm border border-red-200 text-red-600 rounded-lg active:bg-red-50 disabled:opacity-50 min-h-[44px]"
                  >
                    {deletingInst === inst.id ? '删除中...' : '删除'}
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {migrateModal && (
        <div className="fixed inset-0 bg-black/40 flex items-end sm:items-center justify-center z-50 p-0 sm:p-4" onClick={() => !migratingInst && setMigrateModal(null)}>
          <div className="bg-white rounded-t-2xl sm:rounded-2xl p-6 max-w-md w-full shadow-xl" onClick={(e) => e.stopPropagation()}>
            <h2 className="text-lg font-semibold mb-4">迁移实例 #{migrateModal.id} {migrateModal.name}</h2>
            <p className="text-sm text-slate-500 mb-4">选择目标宿主机，将复制工作区并重启容器</p>
            <div className="space-y-2 mb-4">
              {hosts.filter((h) => h.enabled && h.id !== migrateModal.host_id).map((h) => (
                <button
                  key={h.id}
                  onClick={() => handleMigrateInstance(migrateModal, h.id)}
                  disabled={!!migratingInst}
                  className="w-full px-4 py-3 text-left border border-slate-200 rounded-xl hover:bg-slate-50 disabled:opacity-50 flex items-center justify-between"
                >
                  <span>{h.name}</span>
                  <span className="text-slate-500 text-sm">{(instanceImageStatus[h.id]?.instance_count ?? 0)} 实例</span>
                </button>
              ))}
            </div>
            <button type="button" onClick={() => setMigrateModal(null)} className="w-full py-3 border border-slate-300 rounded-xl active:bg-slate-50">
              取消
            </button>
          </div>
        </div>
      )}

      {modal && (
        <div className="fixed inset-0 bg-black/40 flex items-end sm:items-center justify-center z-50 p-0 sm:p-4" onClick={() => setModal(null)}>
          <div className="bg-white rounded-t-2xl sm:rounded-2xl p-6 max-w-md w-full max-h-[90vh] overflow-y-auto shadow-xl" onClick={(e) => e.stopPropagation()}>
            <h2 className="text-lg font-semibold mb-4">{modal === 'add' ? '添加服务器' : '编辑服务器'}</h2>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-2">名称</label>
                <input
                  value={form.name}
                  onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                  className="w-full px-4 py-3 border border-slate-300 rounded-xl"
                  required
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-2">地址 (hostname/IP)</label>
                <input
                  value={form.addr}
                  onChange={(e) => setForm((f) => ({ ...f, addr: e.target.value }))}
                  className="w-full px-4 py-3 border border-slate-300 rounded-xl"
                  required
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-2">SSH 端口</label>
                <input
                  type="number"
                  value={form.ssh_port}
                  onChange={(e) => setForm((f) => ({ ...f, ssh_port: parseInt(e.target.value) || 22 }))}
                  className="w-full px-4 py-3 border border-slate-300 rounded-xl"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-2">SSH 用户</label>
                <input
                  value={form.ssh_user}
                  onChange={(e) => setForm((f) => ({ ...f, ssh_user: e.target.value }))}
                  className="w-full px-4 py-3 border border-slate-300 rounded-xl"
                  required
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-2">SSH 认证（二选一）</label>
                <div className="space-y-3">
                  <div>
                    <label className="block text-xs text-slate-500 mb-1">账号密码</label>
                    <input
                      type="password"
                      value={form.ssh_password || ''}
                      onChange={(e) => setForm((f) => ({ ...f, ssh_password: e.target.value }))}
                      className="w-full px-4 py-3 border border-slate-300 rounded-xl"
                      placeholder="输入 SSH 密码"
                    />
                  </div>
                  <div>
                    <label className="block text-xs text-slate-500 mb-1">或使用私钥</label>
                    <textarea
                      value={form.ssh_key || ''}
                      onChange={(e) => setForm((f) => ({ ...f, ssh_key: e.target.value }))}
                      className="w-full px-4 py-3 border border-slate-300 rounded-xl font-mono text-sm"
                      rows={3}
                      placeholder="-----BEGIN ...（留空则用上面的密码）"
                    />
                  </div>
                </div>
                {modal === 'add' && (
                  <p className="text-xs text-slate-500 mt-1">密码和私钥至少填一项</p>
                )}
                {modal === 'edit' && (
                  <p className="text-xs text-slate-500 mt-1">留空则保留原有认证信息</p>
                )}
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-2">Docker 镜像（可选）</label>
                <input
                  value={form.docker_image}
                  onChange={(e) => setForm((f) => ({ ...f, docker_image: e.target.value }))}
                  className="w-full px-4 py-3 border border-slate-300 rounded-xl"
                  placeholder="openclaw/openclaw"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-2">实例容量上限（0=不限）</label>
                <input
                  type="number"
                  min={0}
                  value={form.instance_capacity ?? 0}
                  onChange={(e) => setForm((f) => ({ ...f, instance_capacity: parseInt(e.target.value) || 0 }))}
                  className="w-full px-4 py-3 border border-slate-300 rounded-xl"
                />
              </div>
              <div className="flex items-center gap-3 min-h-[44px]">
                <input
                  type="checkbox"
                  id="enabled"
                  checked={form.enabled}
                  onChange={(e) => setForm((f) => ({ ...f, enabled: e.target.checked }))}
                  className="w-5 h-5"
                />
                <label htmlFor="enabled" className="text-sm">启用</label>
              </div>
              <div className="flex gap-3 pt-2">
                <button type="submit" disabled={submitting} className="flex-1 py-3 bg-slate-800 text-white rounded-xl active:bg-slate-700 disabled:opacity-50 min-h-[48px] touch-target">
                  {submitting ? '保存中...' : '保存'}
                </button>
                <button type="button" onClick={() => setModal(null)} className="px-6 py-3 border border-slate-300 rounded-xl active:bg-slate-50 min-h-[48px]">
                  取消
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}

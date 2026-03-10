import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { getInstances, createInstance, deleteInstance, getInviteCode, useInviteCode, type Instance, type User } from '../api'

const ADOPT_COST = 100
const MIN_ENERGY = 5

export default function Home({ user }: { user: User | null }) {
  const [instances, setInstances] = useState<Instance[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [creating, setCreating] = useState(false)
  const [newName, setNewName] = useState('')
  const [inviteCode, setInviteCode] = useState('')
  const [myCode, setMyCode] = useState('')
  const [showInvite, setShowInvite] = useState(false)
  const [deleting, setDeleting] = useState<number | null>(null)

  const navigate = useNavigate()

  const loadInstances = () => {
    setLoading(true)
    getInstances()
      .then(setInstances)
      .catch((err) => setError(err instanceof Error ? err.message : '加载失败'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    loadInstances()
  }, [])

  const handleAdopt = async (e: React.FormEvent) => {
    e.preventDefault()
    if (creating) return
    const name = newName.trim() || '小爪'
    if (!user) return
    if (user.energy < ADOPT_COST) {
      setError(`电量不足，领养需要 ${ADOPT_COST} 电量`)
      return
    }
    setCreating(true)
    setError('')
    try {
      const inst = await createInstance(name)
      setInstances((prev) => [inst, ...prev])
      setNewName('')
      navigate(`/instances/${inst.id}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : '领养失败')
    } finally {
      setCreating(false)
    }
  }

  const handleUseInvite = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!inviteCode.trim()) return
    try {
      await useInviteCode(inviteCode.trim())
      setInviteCode('')
      setError('')
      window.location.reload()
    } catch (err) {
      setError(err instanceof Error ? err.message : '邀请码无效或已使用')
    }
  }

  const handleAbandon = async (e: React.MouseEvent, inst: Instance) => {
    e.stopPropagation()
    if (!confirm(`确定弃养「${inst.name}」？弃养后无法恢复。`)) return
    setDeleting(inst.id)
    setError('')
    try {
      await deleteInstance(inst.id)
      setInstances((prev) => prev.filter((i) => i.id !== inst.id))
    } catch (err) {
      setError(err instanceof Error ? err.message : '弃养失败')
    } finally {
      setDeleting(null)
    }
  }

  const handleGetMyCode = async () => {
    try {
      const { code } = await getInviteCode()
      setMyCode(code)
      setShowInvite(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : '获取失败')
    }
  }

  const energyBar = (e: number, max = 100) => {
    const pct = Math.min(100, (e / max) * 100)
    const low = e < MIN_ENERGY
    return (
      <div className="h-2 bg-slate-200 rounded-full overflow-hidden">
        <div
          className={`h-full transition-all ${low ? 'bg-red-500' : e < 30 ? 'bg-amber-500' : 'bg-green-500'}`}
          style={{ width: `${pct}%` }}
        />
      </div>
    )
  }

  return (
    <div className="max-w-2xl mx-auto">
      {/* 电量与邀请 */}
      <div className="mb-4 p-4 bg-white rounded-xl border border-slate-200 flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <span className="text-sm text-slate-600">我的电量</span>
          <span className="text-xl font-bold text-slate-800">{user?.energy ?? 0}</span>
        </div>
        <div className="flex gap-2">
          <button
            onClick={handleGetMyCode}
            className="px-4 py-2 text-sm border border-slate-300 rounded-lg active:bg-slate-50"
          >
            邀请好友
          </button>
          <button
            onClick={() => setShowInvite(!showInvite)}
            className="px-4 py-2 text-sm border border-slate-300 rounded-lg active:bg-slate-50"
          >
            使用邀请码
          </button>
        </div>
      </div>

      {showInvite && (
        <div className="mb-4 p-4 bg-amber-50 rounded-xl border border-amber-200 space-y-4">
          {myCode && (
            <div>
              <p className="text-sm text-slate-700 mb-1">邀请好友注册，双方各得 50 电量</p>
              <p className="font-mono text-lg font-medium">{myCode}</p>
            </div>
          )}
          <form onSubmit={handleUseInvite} className="flex gap-2">
            <input
              value={inviteCode}
              onChange={(e) => setInviteCode(e.target.value)}
              placeholder="输入邀请码兑换电量"
              className="flex-1 px-3 py-2 border rounded-lg"
            />
            <button type="submit" className="px-4 py-2 bg-slate-800 text-white rounded-lg">
              兑换
            </button>
          </form>
        </div>
      )}

      {/* 领养新宠物 */}
      <div className="mb-6">
        <h2 className="text-lg font-semibold text-slate-800 mb-3">领养 OpenClaw</h2>
        <form onSubmit={handleAdopt} className="flex flex-col sm:flex-row gap-3">
          <input
            type="text"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            placeholder="给宠物起个名字"
            className="flex-1 px-4 py-3 border border-slate-300 rounded-xl"
          />
          <button
            type="submit"
            disabled={creating || (user?.energy ?? 0) < ADOPT_COST}
            className="px-6 py-3 bg-slate-800 text-white rounded-xl disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {creating ? '领养中，请稍候（约 1–2 分钟）...' : `领养 (${ADOPT_COST} 电量)`}
          </button>
        </form>
        {(user?.energy ?? 0) < ADOPT_COST && (
          <p className="mt-2 text-sm text-amber-600">电量不足，可通过邀请好友获取</p>
        )}
      </div>

      {error && (
        <p className="mb-4 text-sm text-red-600 bg-red-50 p-3 rounded-lg">{error}</p>
      )}

      {/* 宠舍 */}
      <h2 className="text-lg font-semibold text-slate-800 mb-3">我的宠舍</h2>
      {loading ? (
        <p className="text-slate-500 py-8">加载中...</p>
      ) : instances.length === 0 ? (
        <div className="text-center py-12 bg-slate-50 rounded-xl">
          <p className="text-slate-500 mb-2">暂无宠物</p>
          <p className="text-sm text-slate-400">领养一只 OpenClaw 开始对话吧</p>
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2">
          {instances.map((inst) => (
            <div
              key={inst.id}
              onClick={() => navigate(`/instances/${inst.id}`)}
              className="bg-white border border-slate-200 rounded-xl p-4 active:bg-slate-50 cursor-pointer transition-colors relative"
            >
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0 flex-1">
                  <p className="font-medium text-slate-800 truncate">{inst.name}</p>
                  <div className="mt-2">
                    <div className="flex justify-between text-xs text-slate-500 mb-1">
                      <span>电量</span>
                      <span className={inst.energy < MIN_ENERGY ? 'text-red-600' : ''}>
                        {inst.energy}
                      </span>
                    </div>
                    {energyBar(inst.energy)}
                  </div>
                </div>
                <div className="flex items-center gap-2 flex-shrink-0">
                  <span
                    className={`px-2.5 py-1 text-xs rounded-full ${
                      inst.status === 'running'
                        ? 'bg-green-100 text-green-800'
                        : inst.status === 'creating'
                        ? 'bg-amber-100 text-amber-800'
                        : inst.status === 'error'
                        ? 'bg-red-100 text-red-800'
                        : 'bg-slate-100 text-slate-700'
                    }`}
                  >
                    {inst.status === 'running' ? '在线' : inst.status === 'creating' ? '创建中' : inst.status === 'error' ? '异常' : inst.status}
                  </span>
                  <button
                    onClick={(e) => handleAbandon(e, inst)}
                    disabled={!!deleting}
                    className="px-2 py-1 text-xs text-red-600 border border-red-200 rounded-lg active:bg-red-50 disabled:opacity-50"
                    title="弃养"
                  >
                    {deleting === inst.id ? '弃养中...' : '弃养'}
                  </button>
                </div>
              </div>
              {inst.energy < MIN_ENERGY && inst.status === 'running' && (
                <p className="mt-2 text-xs text-amber-600">电量不足，无法对话</p>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

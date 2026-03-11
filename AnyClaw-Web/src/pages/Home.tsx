import { useState, useEffect } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { getInstances, createInstance, deleteInstance, getInviteCode, useInviteCode, type Instance, type User } from '../api'

const ADOPT_COST = 100

export default function Home({ user, onRefresh }: { user: User | null; onRefresh?: () => void }) {
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
      setError(`金币不足，领养需要 ${ADOPT_COST} 金币`)
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
    if (!confirm(`确定弃养「${inst.name}」？弃养后无法恢复，系统将删除该宠物的所有聊天记录。`)) return
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

  return (
    <div className="max-w-2xl mx-auto">
      {/* 金币与邀请 - 移动端极简 */}
      <div className="mb-4 p-3 sm:p-4 bg-white rounded-xl border border-slate-200 flex flex-wrap items-center justify-between gap-2 sm:gap-3">
        <div className="flex items-center gap-2 sm:gap-3">
          <span className="text-sm text-slate-600 hidden sm:inline">我的金币</span>
          <span className="text-xl font-bold text-slate-800">🪙 {user?.energy ?? 0}</span>
        </div>
        <div className="flex gap-1.5 sm:gap-2">
          <Link
            to="/usage"
            className="px-3 py-1.5 sm:px-4 sm:py-2 text-sm border border-slate-300 rounded-lg active:bg-slate-50"
          >
            消耗
          </Link>
          <Link
            to="/recharge"
            className="px-3 py-1.5 sm:px-4 sm:py-2 text-sm font-medium bg-amber-500 text-amber-950 rounded-lg hover:bg-amber-400 active:bg-amber-600"
          >
            充值
          </Link>
          <button
            onClick={handleGetMyCode}
            className="px-3 py-1.5 sm:px-4 sm:py-2 text-sm border border-slate-300 rounded-lg active:bg-slate-50"
          >
            邀请
          </button>
          <button
            onClick={() => setShowInvite(!showInvite)}
            className="px-3 py-1.5 sm:px-4 sm:py-2 text-sm border border-slate-300 rounded-lg active:bg-slate-50"
          >
            兑换
          </button>
        </div>
      </div>

      {showInvite && (
        <div className="mb-4 p-3 sm:p-4 bg-amber-50 rounded-xl border border-amber-200 space-y-3 sm:space-y-4">
          <p className="text-sm text-slate-700">邀请好友注册，双方各得金币；好友充值时你可获 5% 返利</p>
          {myCode && (
            <>
              <div>
                <p className="text-xs text-slate-500 mb-1">邀请链接</p>
                <div className="flex gap-2">
                  <input
                    readOnly
                    value={`${typeof window !== 'undefined' ? window.location.origin : ''}/register?invite=${myCode}`}
                    className="flex-1 px-3 py-2 border border-slate-300 rounded-lg text-sm bg-white min-w-0"
                  />
                  <button
                    type="button"
                    onClick={() => {
                      const url = `${window.location.origin}/register?invite=${myCode}`
                      navigator.clipboard?.writeText(url).then(() => alert('已复制'))
                    }}
                    className="px-3 py-2 text-sm bg-slate-800 text-white rounded-lg flex-shrink-0"
                  >
                    复制
                  </button>
                </div>
              </div>
              <div className="flex flex-col sm:flex-row items-center gap-4">
                <div className="bg-white p-2 rounded-lg">
                  <img
                    src={`https://api.qrserver.com/v1/create-qr-code/?size=120x120&data=${encodeURIComponent(`${window.location.origin}/register?invite=${myCode}`)}`}
                    alt="邀请二维码"
                  />
                </div>
                <div className="flex-1 min-w-0">
                  <p className="text-xs text-slate-500 mb-1">邀请码</p>
                  <p className="font-mono text-base font-medium break-all">{myCode}</p>
                </div>
              </div>
            </>
          )}
          <form onSubmit={handleUseInvite} className="flex gap-2">
            <input
              value={inviteCode}
              onChange={(e) => setInviteCode(e.target.value)}
              placeholder="邀请码"
              className="flex-1 px-3 py-2 border rounded-lg text-base min-w-0"
            />
            <button type="submit" className="px-4 py-2 bg-slate-800 text-white rounded-lg flex-shrink-0">
              兑换
            </button>
          </form>
        </div>
      )}

      {/* 领养新宠物 */}
      <div className="mb-6">
        <div className="flex items-center gap-3 mb-2 sm:mb-3">
          <img src="/10002.svg" alt="" className="w-12 h-12 sm:w-14 sm:h-14 flex-shrink-0" aria-hidden />
          <div>
            <h2 className="text-base sm:text-lg font-semibold text-slate-800">领养 OpenClaw</h2>
            <p className="text-sm text-slate-500">每只宠物都有唯一的灵魂，擅长复杂任务、拥有超长记忆，回答会稍慢一些～</p>
          </div>
        </div>
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
            {creating ? '领养中，请稍候（约 1–2 分钟）...' : `领养 (${ADOPT_COST} 金币)`}
          </button>
        </form>
        {(user?.energy ?? 0) < ADOPT_COST && (
          <p className="mt-2 text-sm text-amber-600">金币不足，可通过邀请好友获取</p>
        )}
      </div>

      {error && (
        <p className="mb-4 text-sm text-red-600 bg-red-50 p-3 rounded-lg">{error}</p>
      )}

      {/* 宠舍 */}
      <h2 className="text-base sm:text-lg font-semibold text-slate-800 mb-2 sm:mb-3 hidden sm:block">我的宠舍</h2>
      {loading ? (
        <p className="text-slate-500 py-8">加载中...</p>
      ) : instances.length === 0 ? (
        <div className="text-center py-12 bg-slate-50 rounded-xl">
          <img src="/10003.png" alt="" className="w-24 h-24 mx-auto mb-3 object-contain" aria-hidden />
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
                <div className="flex items-center gap-3 min-w-0 flex-1">
                  <img src="/10001.png" alt="" className="w-10 h-10 flex-shrink-0 object-contain" aria-hidden />
                  <p className="font-medium text-slate-800 truncate">{inst.name}</p>
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
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

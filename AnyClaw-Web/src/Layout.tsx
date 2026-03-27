import { useState } from 'react'
import { useLocation } from 'react-router-dom'
import type { User } from './api'
import { SafeLink } from './components/SafeLink'

interface LayoutProps {
  user: User | null;
  onLogout: () => void;
  children: React.ReactNode;
}

export default function Layout({ user, onLogout, children }: LayoutProps) {
  const [showQqModal, setShowQqModal] = useState(false)
  const loc = useLocation()
  const isHome = loc.pathname === '/'
  const isRecharge = loc.pathname === '/recharge'
  const isUsage = loc.pathname === '/usage'
  const isHosts = loc.pathname.startsWith('/admin/hosts')
  const isEnergy = loc.pathname.startsWith('/admin/energy')
  const isConfig = loc.pathname.startsWith('/admin/config')
  const isStats = loc.pathname.startsWith('/admin/stats')
  // 仅 `/instances/:id` 聊天全屏；`/instances/:id/collab` 保留顶栏
  const isChat = /^\/instances\/[^/]+$/.test(loc.pathname)
  const isHelp = loc.pathname === '/help'

  return (
    <div className={`min-h-screen bg-slate-50 flex flex-col ${isChat ? 'pb-0' : 'pb-16 sm:pb-0'}`}>
      <header className={`sticky top-0 z-20 bg-white/95 backdrop-blur border-b border-slate-200 px-4 py-3 flex items-center justify-between ${isChat ? 'hidden' : ''}`}>
        <SafeLink to="/" className="flex items-center gap-2 text-lg font-semibold text-slate-800 active:opacity-70">
          <img src="/10002.svg" alt="" className="w-8 h-8" aria-hidden />
          <span className="hidden sm:inline">OpenClaw</span>
        </SafeLink>
        {user && (
          <div className="hidden sm:flex items-center gap-2">
            <SafeLink to="/usage" className="px-2.5 py-1.5 rounded-lg text-slate-600 hover:bg-slate-100 text-sm">消耗</SafeLink>
            <SafeLink to="/recharge" className="px-3 py-1.5 rounded-lg bg-amber-100 text-amber-800 font-medium hover:bg-amber-200 active:bg-amber-300 text-sm">
              🪙 {user.energy ?? 0} · 充值
            </SafeLink>
          </div>
        )}
        <nav className="flex items-center gap-2 sm:gap-4">
          {user?.role === 'admin' && (
            <>
              <SafeLink to="/admin/config" className={`hidden sm:inline px-3 py-2 text-sm rounded-lg -m-1 ${isConfig ? 'text-slate-800 font-medium bg-slate-100' : 'text-slate-600 active:bg-slate-100'}`}>配置</SafeLink>
              <SafeLink to="/admin/stats" className={`hidden sm:inline px-3 py-2 text-sm rounded-lg -m-1 ${isStats ? 'text-slate-800 font-medium bg-slate-100' : 'text-slate-600 active:bg-slate-100'}`}>监控</SafeLink>
              <SafeLink to="/admin/energy" className={`hidden sm:inline px-3 py-2 text-sm rounded-lg -m-1 ${isEnergy ? 'text-slate-800 font-medium bg-slate-100' : 'text-slate-600 active:bg-slate-100'}`}>用户</SafeLink>
              <SafeLink to="/admin/hosts" className={`hidden sm:inline px-3 py-2 text-sm rounded-lg -m-1 ${isHosts ? 'text-slate-800 font-medium bg-slate-100' : 'text-slate-600 active:bg-slate-100'}`}>服务器</SafeLink>
            </>
          )}
          {user && <span className="hidden md:inline text-sm text-slate-500 truncate max-w-[120px]">{user.email}</span>}
          {user && (
            <SafeLink to="/recharge" className="sm:hidden px-2 py-1.5 rounded-lg bg-amber-100 text-amber-800 text-sm font-medium">
              🪙 {user.energy ?? 0}
            </SafeLink>
          )}
          <SafeLink
            to="/help"
            className={`px-3 py-2 text-sm rounded-lg -m-1 touch-target ${isHelp ? 'text-slate-800 font-medium bg-slate-100' : 'text-slate-600 active:bg-slate-100'}`}
          >
            帮助
          </SafeLink>
          <button
            onClick={onLogout}
            className="px-3 py-2 text-sm text-slate-600 active:bg-slate-100 rounded-lg -m-1 touch-target"
          >
            退出
          </button>
        </nav>
      </header>
      <main className="flex-1 p-4 sm:p-6">
        {children}
      </main>
      {/* 交流群链接 */}
      {!isChat && (
        <footer className="py-2 pb-16 sm:pb-2 text-center flex flex-col items-center gap-1">
          <div className="flex items-center gap-3">
            <SafeLink to="/help" className="text-sm text-slate-500 hover:text-slate-700 active:opacity-70">
              帮助
            </SafeLink>
            <span className="text-slate-300">|</span>
            <button
              type="button"
              onClick={() => setShowQqModal(true)}
              className="text-sm text-slate-500 hover:text-slate-700 active:opacity-70"
            >
              加入 OpenClaw 探索群
            </button>
          </div>
        </footer>
      )}
      {showQqModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => setShowQqModal(false)}>
          <div className="bg-white rounded-xl p-6 shadow-xl max-w-sm mx-4" onClick={(e) => e.stopPropagation()}>
            <p className="text-base font-medium text-slate-800 mb-3">OpenClaw 探索群 群号: 1049101776</p>
            <img src="/qqgroup.jpg" alt="OpenClaw探索群" className="w-48 h-48 mx-auto rounded-lg" />
            <p className="text-sm text-slate-500 mt-3">扫一扫加入群聊</p>
            <button
              type="button"
              onClick={() => setShowQqModal(false)}
              className="mt-4 w-full py-2 text-sm text-slate-600 border border-slate-200 rounded-lg hover:bg-slate-50"
            >
              关闭
            </button>
          </div>
        </div>
      )}
      {/* 移动端底部导航 - 仅手机端，聊天页隐藏，极简 */}
      <nav className={`fixed bottom-0 left-0 right-0 sm:hidden bg-white/95 backdrop-blur border-t border-slate-200 flex justify-around items-center z-20 pb-safe ${isChat ? 'hidden' : ''}`}>
        <SafeLink to="/" className={`flex-1 flex flex-col items-center py-2 px-1 active:bg-slate-50 ${isHome ? 'text-slate-800 font-medium' : 'text-slate-500'}`}>
          <img src="/10001.png" alt="" className="w-5 h-5 object-contain" aria-hidden />
          <span className="text-[10px] mt-0.5">首页</span>
        </SafeLink>
        {user && (
          <>
            <SafeLink to="/usage" className={`flex-1 flex flex-col items-center py-2 px-1 active:bg-slate-50 ${isUsage ? 'text-slate-800 font-medium' : 'text-slate-500'}`}>
              <span className="text-base">📊</span>
              <span className="text-[10px] mt-0.5">消耗</span>
            </SafeLink>
            <SafeLink to="/recharge" className={`flex-1 flex flex-col items-center py-2 px-1 active:bg-slate-50 ${isRecharge ? 'text-amber-600 font-medium' : 'text-slate-500'}`}>
              <span className="text-base">🪙</span>
              <span className="text-[10px] mt-0.5">充值</span>
            </SafeLink>
          </>
        )}
        {user?.role === 'admin' && (
          <>
            <SafeLink to="/admin/config" className={`flex-1 flex flex-col items-center py-2 px-1 active:bg-slate-50 ${isConfig ? 'text-slate-800 font-medium' : 'text-slate-500'}`}>
              <span className="text-base">🤖</span>
              <span className="text-[10px] mt-0.5">AI</span>
            </SafeLink>
            <SafeLink to="/admin/stats" className={`flex-1 flex flex-col items-center py-2 px-1 active:bg-slate-50 ${isStats ? 'text-slate-800 font-medium' : 'text-slate-500'}`}>
              <span className="text-base">📈</span>
              <span className="text-[10px] mt-0.5">监控</span>
            </SafeLink>
            <SafeLink to="/admin/energy" className={`flex-1 flex flex-col items-center py-2 px-1 active:bg-slate-50 ${isEnergy ? 'text-slate-800 font-medium' : 'text-slate-500'}`}>
              <span className="text-base">👥</span>
              <span className="text-[10px] mt-0.5">用户</span>
            </SafeLink>
            <SafeLink to="/admin/hosts" className={`flex-1 flex flex-col items-center py-2 px-1 active:bg-slate-50 ${isHosts ? 'text-slate-800 font-medium' : 'text-slate-500'}`}>
              <span className="text-base">🖥</span>
              <span className="text-[10px] mt-0.5">服务器</span>
            </SafeLink>
          </>
        )}
      </nav>
    </div>
  );
}

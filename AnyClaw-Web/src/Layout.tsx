import { Link, useLocation } from 'react-router-dom'
import type { User } from './api'

interface LayoutProps {
  user: User | null;
  onLogout: () => void;
  children: React.ReactNode;
}

export default function Layout({ user, onLogout, children }: LayoutProps) {
  const loc = useLocation()
  const isHome = loc.pathname === '/'
  const isHosts = loc.pathname.startsWith('/admin/hosts')
  const isEnergy = loc.pathname.startsWith('/admin/energy')
  const isConfig = loc.pathname.startsWith('/admin/config')
  const isStats = loc.pathname.startsWith('/admin/stats')

  return (
    <div className="min-h-screen bg-slate-50 flex flex-col pb-16 sm:pb-0">
      <header className="sticky top-0 z-20 bg-white/95 backdrop-blur border-b border-slate-200 px-4 py-3 flex items-center justify-between">
        <Link to="/" className="text-lg font-semibold text-slate-800 active:opacity-70">
          OpenClaw
        </Link>
        {user && (
          <span className="text-sm text-amber-600 font-medium">⚡ {user.energy ?? 0}</span>
        )}
        <nav className="flex items-center gap-2 sm:gap-4">
          {user?.role === 'admin' && (
            <>
              <Link to="/admin/config" className={`px-3 py-2 text-sm rounded-lg -m-1 ${isConfig ? 'text-slate-800 font-medium bg-slate-100' : 'text-slate-600 active:bg-slate-100'}`}>
                AI配置
              </Link>
              <Link to="/admin/stats" className={`px-3 py-2 text-sm rounded-lg -m-1 ${isStats ? 'text-slate-800 font-medium bg-slate-100' : 'text-slate-600 active:bg-slate-100'}`}>
                监控
              </Link>
              <Link to="/admin/energy" className={`px-3 py-2 text-sm rounded-lg -m-1 ${isEnergy ? 'text-slate-800 font-medium bg-slate-100' : 'text-slate-600 active:bg-slate-100'}`}>
                电量
              </Link>
              <Link to="/admin/hosts" className={`px-3 py-2 text-sm rounded-lg -m-1 ${isHosts ? 'text-slate-800 font-medium bg-slate-100' : 'text-slate-600 active:bg-slate-100'}`}>
                服务器
              </Link>
            </>
          )}
          {user && (
            <span className="hidden sm:inline text-sm text-slate-500 truncate max-w-[120px]">{user.email}</span>
          )}
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
      {/* 移动端底部导航 */}
      <nav className="fixed bottom-0 left-0 right-0 sm:hidden bg-white/95 backdrop-blur border-t border-slate-200 flex justify-around items-center z-20 pb-safe">
        <Link
          to="/"
          className={`flex-1 flex flex-col items-center py-3 px-2 active:bg-slate-50 ${isHome ? 'text-slate-800 font-medium' : 'text-slate-500'}`}
        >
          <span className="text-lg">🏠</span>
          <span className="text-xs mt-0.5">首页</span>
        </Link>
        {user?.role === 'admin' && (
          <>
            <Link
              to="/admin/config"
              className={`flex-1 flex flex-col items-center py-3 px-2 active:bg-slate-50 ${isConfig ? 'text-slate-800 font-medium' : 'text-slate-500'}`}
            >
              <span className="text-lg">🤖</span>
              <span className="text-xs mt-0.5">AI</span>
            </Link>
            <Link
              to="/admin/stats"
              className={`flex-1 flex flex-col items-center py-3 px-2 active:bg-slate-50 ${isStats ? 'text-slate-800 font-medium' : 'text-slate-500'}`}
            >
              <span className="text-lg">📊</span>
              <span className="text-xs mt-0.5">监控</span>
            </Link>
            <Link
              to="/admin/energy"
              className={`flex-1 flex flex-col items-center py-3 px-2 active:bg-slate-50 ${isEnergy ? 'text-slate-800 font-medium' : 'text-slate-500'}`}
            >
              <span className="text-lg">⚡</span>
              <span className="text-xs mt-0.5">电量</span>
            </Link>
            <Link
              to="/admin/hosts"
              className={`flex-1 flex flex-col items-center py-3 px-2 active:bg-slate-50 ${isHosts ? 'text-slate-800 font-medium' : 'text-slate-500'}`}
            >
              <span className="text-lg">🖥️</span>
              <span className="text-xs mt-0.5">服务器</span>
            </Link>
          </>
        )}
      </nav>
    </div>
  );
}

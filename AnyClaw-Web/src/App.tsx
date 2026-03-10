import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { useState, useEffect } from 'react'
import { getMe, getSetupStatus, clearToken, isAuthenticated, type User } from './api'

const refreshUser = async (setUser: (u: User | null) => void) => {
  try {
    const u = await getMe()
    setUser(u)
  } catch {
    setUser(null)
  }
}
import Layout from './Layout'
import Login from './pages/Login'
import Register from './pages/Register'
import Home from './pages/Home'
import Chat from './pages/Chat'
import Hosts from './pages/Hosts'
import Energy from './pages/Energy'
import AdminConfig from './pages/AdminConfig'
import AdminStats from './pages/AdminStats'
import Setup from './pages/Setup'

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  if (!isAuthenticated()) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}

function PublicRoute({ children }: { children: React.ReactNode }) {
  if (isAuthenticated()) {
    return <Navigate to="/" replace />;
  }
  return <>{children}</>;
}

function AdminRoute({ children, user }: { children: React.ReactNode; user: User | null }) {
  if (!isAuthenticated()) {
    return <Navigate to="/login" replace />;
  }
  if (user?.role !== 'admin') {
    return <Navigate to="/" replace />;
  }
  return <>{children}</>;
}

export default function App() {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [setupRequired, setSetupRequired] = useState<boolean | null>(null);

  useEffect(() => {
    getSetupStatus()
      .then(({ configured }: { configured: boolean }) => {
        setSetupRequired(!configured);
        if (!configured) {
          setLoading(false);
          return;
        }
        if (!isAuthenticated()) {
          setLoading(false);
          return;
        }
        getMe()
          .then(setUser)
          .catch(() => {
            clearToken();
            setUser(null);
          })
          .finally(() => setLoading(false));
      })
      .catch(() => {
        setSetupRequired(true);
        setLoading(false);
      });
  }, []);

  const handleLogin = (u: User) => setUser(u);
  const handleLogout = () => {
    clearToken();
    setUser(null);
  };

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-slate-50">
        <div className="text-slate-500">Loading...</div>
      </div>
    );
  }

  if (setupRequired) {
    return (
      <BrowserRouter>
        <Routes>
          <Route path="/setup" element={<Setup />} />
          <Route path="*" element={<Navigate to="/setup" replace />} />
        </Routes>
      </BrowserRouter>
    );
  }

  return (
    <BrowserRouter>
      <Routes>
        <Route path="/setup" element={<Navigate to="/" replace />} />
        <Route path="/login" element={
          <PublicRoute>
            <Login onLogin={handleLogin} />
          </PublicRoute>
        } />
        <Route path="/register" element={
          <PublicRoute>
            <Register onRegister={handleLogin} />
          </PublicRoute>
        } />
        <Route path="/" element={
          <ProtectedRoute>
            <Layout user={user} onLogout={handleLogout}>
              <Home user={user} onRefresh={() => refreshUser(setUser)} />
            </Layout>
          </ProtectedRoute>
        } />
        <Route path="/instances/:id" element={
          <ProtectedRoute>
            <Layout user={user} onLogout={handleLogout}>
              <Chat />
            </Layout>
          </ProtectedRoute>
        } />
        <Route path="/admin/config" element={
          <AdminRoute user={user}>
            <Layout user={user} onLogout={handleLogout}>
              <AdminConfig />
            </Layout>
          </AdminRoute>
        } />
        <Route path="/admin/stats" element={
          <AdminRoute user={user}>
            <Layout user={user} onLogout={handleLogout}>
              <AdminStats />
            </Layout>
          </AdminRoute>
        } />
        <Route path="/admin/hosts" element={
          <AdminRoute user={user}>
            <Layout user={user} onLogout={handleLogout}>
              <Hosts />
            </Layout>
          </AdminRoute>
        } />
        <Route path="/admin/energy" element={
          <AdminRoute user={user}>
            <Layout user={user} onLogout={handleLogout}>
              <Energy />
            </Layout>
          </AdminRoute>
        } />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  );
}

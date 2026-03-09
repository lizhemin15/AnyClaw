import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { useState, useEffect } from 'react'
import { getMe, getSetupStatus, clearToken, isAuthenticated, type User } from './api'
import Layout from './Layout'
import Login from './pages/Login'
import Register from './pages/Register'
import Home from './pages/Home'
import Chat from './pages/Chat'
import Hosts from './pages/Hosts'
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
              <Home />
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
        <Route path="/admin/hosts" element={
          <AdminRoute user={user}>
            <Layout user={user} onLogout={handleLogout}>
              <Hosts />
            </Layout>
          </AdminRoute>
        } />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  );
}

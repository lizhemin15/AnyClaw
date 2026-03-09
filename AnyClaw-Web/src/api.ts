const API_BASE = (import.meta.env.VITE_API_URL || '').trim() || (import.meta.env.DEV ? 'http://localhost:8080' : '');

export function getToken(): string | null {
  return localStorage.getItem('token');
}

export function setToken(token: string): void {
  localStorage.setItem('token', token);
}

export function clearToken(): void {
  localStorage.removeItem('token');
}

export function isAuthenticated(): boolean {
  return !!getToken();
}

export interface User {
  id: number;
  email: string;
  role: string;
  created_at: string;
}

export interface Instance {
  id: number;
  user_id: number;
  name: string;
  status: string;
  created_at: string;
}

export interface LoginResponse {
  access_token: string;
  user: User;
}

async function fetchApi<T>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const token = getToken();
  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    ...options.headers,
  };
  if (token) {
    (headers as Record<string, string>)['Authorization'] = `Bearer ${token}`;
  }

  const res = await fetch(`${API_BASE}${path}`, { ...options, headers });
  const text = await res.text();
  let data: unknown;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    throw new Error(text || res.statusText || 'Request failed');
  }

  if (!res.ok) {
    const err = data as { error?: string };
    throw new Error(err?.error || text || res.statusText);
  }
  return data as T;
}

export async function login(email: string, password: string): Promise<LoginResponse> {
  return fetchApi<LoginResponse>('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  });
}

export async function register(email: string, password: string): Promise<LoginResponse> {
  return fetchApi<LoginResponse>('/auth/register', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  });
}

export async function getMe(): Promise<User> {
  return fetchApi<User>('/me');
}

export async function getInstances(): Promise<Instance[]> {
  const data = await fetchApi<Instance[] | null>('/instances');
  return Array.isArray(data) ? data : [];
}

export async function createInstance(name: string): Promise<Instance> {
  return fetchApi<Instance>('/instances', {
    method: 'POST',
    body: JSON.stringify({ name }),
  });
}

export function getWebSocketUrl(instanceId: number): string {
  const base = API_BASE
    ? API_BASE.replace(/^http/, 'ws')
    : (typeof window !== 'undefined'
        ? `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}`
        : 'ws://localhost:8080');
  const token = getToken();
  const url = `${base}/instances/${instanceId}/ws`;
  return token ? `${url}?token=${encodeURIComponent(token)}` : url;
}

export interface Host {
  id: string;
  name: string;
  addr: string;
  ssh_port: number;
  ssh_user: string;
  docker_image?: string;
  enabled: boolean;
  status: string;
  last_check_at?: string;
  created_at: string;
}

export async function getHosts(): Promise<Host[]> {
  const data = await fetchApi<Host[] | null>('/admin/hosts');
  return Array.isArray(data) ? data : [];
}

export interface CreateHostRequest {
  name: string;
  addr: string;
  ssh_port?: number;
  ssh_user: string;
  ssh_key?: string;
  ssh_password?: string;
  docker_image?: string;
  enabled?: boolean;
}

export interface UpdateHostRequest {
  name: string;
  addr: string;
  ssh_port?: number;
  ssh_user: string;
  ssh_key?: string;
  ssh_password?: string;
  docker_image?: string;
  enabled?: boolean;
}

export async function createHost(data: CreateHostRequest): Promise<Host> {
  return fetchApi<Host>('/admin/hosts', {
    method: 'POST',
    body: JSON.stringify({
      name: data.name,
      addr: data.addr,
      ssh_port: data.ssh_port ?? 22,
      ssh_user: data.ssh_user,
      ssh_key: data.ssh_key || '',
      ssh_password: data.ssh_password || '',
      docker_image: data.docker_image || '',
      enabled: data.enabled ?? true,
    }),
  });
}

export async function updateHost(id: string, data: UpdateHostRequest): Promise<Host> {
  return fetchApi<Host>(`/admin/hosts/${id}`, {
    method: 'PUT',
    body: JSON.stringify({
      name: data.name,
      addr: data.addr,
      ssh_port: data.ssh_port ?? 22,
      ssh_user: data.ssh_user,
      ssh_key: data.ssh_key || '',
      ssh_password: data.ssh_password || '',
      docker_image: data.docker_image || '',
      enabled: data.enabled ?? true,
    }),
  });
}

export async function deleteHost(id: string): Promise<void> {
  return fetchApi(`/admin/hosts/${id}`, { method: 'DELETE' });
}

export async function checkHostStatus(id: string): Promise<{ status: string }> {
  return fetchApi<{ status: string }>(`/admin/hosts/${id}/check`, { method: 'POST' });
}

export async function getSetupStatus(): Promise<{ configured: boolean }> {
  const res = await fetch(`${API_BASE}/api/setup/status`);
  const data = await res.json();
  return data;
}

export async function setupDatabase(data: {
  host: string;
  port?: number;
  user: string;
  password: string;
  database: string;
}): Promise<void> {
  return fetchApi('/api/setup/db', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function setupAdmin(data: { email: string; password: string }): Promise<void> {
  return fetchApi('/api/setup/admin', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

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
  energy: number;
  created_at: string;
}

export interface Instance {
  id: number;
  user_id: number;
  name: string;
  status: string;
  energy: number;
  daily_consume: number;
  zero_energy_since?: string;
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
    if (res.status === 401) {
      clearToken();
      const returnTo = encodeURIComponent(window.location.pathname + window.location.search);
      window.location.href = `/login?expired=1&return_to=${returnTo}`;
      throw new Error('登录已过期，请重新登录');
    }
    const err = data as { error?: string };
    throw new Error(err?.error || text || res.statusText);
  }
  return data as T;
}

export async function getAuthConfig(): Promise<{ email_verification_required: boolean }> {
  return fetchApi<{ email_verification_required: boolean }>('/auth/config');
}

export async function sendVerificationCode(email: string): Promise<void> {
  await fetchApi('/auth/send-code', {
    method: 'POST',
    body: JSON.stringify({ email }),
  });
}

export async function login(email: string, password: string): Promise<LoginResponse> {
  return fetchApi<LoginResponse>('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  });
}

export async function register(
  email: string,
  password: string,
  options?: { code?: string; inviteCode?: string }
): Promise<LoginResponse> {
  const body: Record<string, string> = { email, password, invite_code: options?.inviteCode || '' };
  if (options?.code) body.code = options.code;
  return fetchApi<LoginResponse>('/auth/register', {
    method: 'POST',
    body: JSON.stringify(body),
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

export async function getInstance(id: number): Promise<Instance> {
  return fetchApi<Instance>(`/instances/${id}`);
}

export async function deleteInstance(id: number): Promise<void> {
  await fetchApi(`/instances/${id}`, { method: 'DELETE' });
}

export async function feedInstance(id: number, amount: number): Promise<Instance> {
  return fetchApi<Instance>(`/instances/${id}/feed`, {
    method: 'POST',
    body: JSON.stringify({ amount }),
  });
}

export interface ChatMessage {
  id: number;
  instance_id: number;
  role: string;
  content: string;
  created_at: string;
}

export async function getMessages(
  instanceId: number,
  limit = 20,
  before?: number
): Promise<{ messages: ChatMessage[] }> {
  let url = `/instances/${instanceId}/messages?limit=${limit}`;
  if (before != null) url += `&before=${before}`;
  return fetchApi<{ messages: ChatMessage[] }>(url);
}

export function getWebSocketUrl(instanceId: number): string {
  let base: string;
  if (API_BASE) {
    base = API_BASE.replace(/^http/, 'ws');
    // 页面为 HTTPS 时使用 wss，避免混合内容被拦截
    if (typeof window !== 'undefined' && window.location.protocol === 'https:' && API_BASE.startsWith('http://')) {
      base = API_BASE.replace(/^http:\/\//, 'wss://');
    }
  } else {
    base = typeof window !== 'undefined'
      ? `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}`
      : 'ws://localhost:8080';
  }
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

export async function getHostUpdateStatus(id: string): Promise<{
  update_available: boolean;
  script_exists: boolean;
  current_digest?: string;
  latest_digest?: string;
  message?: string;
}> {
  return fetchApi(`/admin/hosts/${id}/update-status`);
}

export async function updateHostMainService(id: string): Promise<{ ok: boolean; message: string; output?: string }> {
  return fetchApi<{ ok: boolean; message: string; output?: string }>(`/admin/hosts/${id}/update`, { method: 'POST' });
}

export interface AdminInstance {
  id: number;
  user_id: number;
  name: string;
  status: string;
  energy: number;
  daily_consume: number;
  container_id?: string;
  host_id?: string;
  created_at: string;
  user_email: string;
  host_name: string;
}

export async function getAdminInstances(): Promise<AdminInstance[]> {
  const data = await fetchApi<AdminInstance[] | null>('/admin/instances');
  return Array.isArray(data) ? data : [];
}

export async function adminDeleteInstance(id: number): Promise<void> {
  await fetchApi(`/admin/instances/${id}`, { method: 'DELETE' });
}

export interface UserWithInstances extends User {
  instance_count: number;
}

export async function getAdminUsers(): Promise<UserWithInstances[]> {
  const data = await fetchApi<UserWithInstances[] | null>('/admin/energy/users');
  return Array.isArray(data) ? data : [];
}

export interface ModelEntry {
  id: string;
  name: string;
  enabled: boolean;
}

export interface Channel {
  id: string;
  name: string;
  api_key: string;
  api_base: string;
  enabled: boolean;
  models: ModelEntry[];
}

export interface SMTPConfig {
  host: string;
  port: number;
  user: string;
  pass: string;
  from: string;
}

export interface PaymentPlan {
  id: string;
  name: string;
  energy: number;
  price_cny: number;
  sort: number;
}

export interface AlipayConfig {
  enabled: boolean;
  app_id: string;
  private_key: string;
  alipay_public_key: string;
  is_sandbox: boolean;
}

export interface WechatConfig {
  enabled: boolean;
  app_id: string;
  mch_id: string;
  api_v3_key: string;
  serial_no: string;
  private_key: string;
}

export interface PaymentConfig {
  alipay?: AlipayConfig;
  wechat?: WechatConfig;
  plans: PaymentPlan[];
}

export interface EnergyConfig {
  tokens_per_energy: number;
  adopt_cost: number;
  daily_consume: number;
  min_energy_for_task: number;
  zero_days_to_delete: number;
  invite_reward: number;
  new_user_energy: number;
  invite_commission_rate: number;
}

export interface AdminConfig {
  channels: Channel[];
  smtp?: SMTPConfig;
  payment?: PaymentConfig;
  energy?: EnergyConfig;
}

export async function getAdminConfig(): Promise<AdminConfig> {
  return fetchApi<AdminConfig>('/admin/config');
}

export async function putAdminConfig(config: AdminConfig): Promise<void> {
  await fetchApi('/admin/config', {
    method: 'PUT',
    body: JSON.stringify(config),
  });
}

export async function testSMTPConfig(params?: Partial<SMTPConfig>): Promise<{ ok: boolean; message: string }> {
  return fetchApi<{ ok: boolean; message: string }>('/admin/config/test-smtp', {
    method: 'POST',
    body: JSON.stringify(params || {}),
  });
}

export async function testChannelConfig(params: {
  channel_id?: string;
  model?: string;
  api_base?: string;
  api_key?: string;
}): Promise<{ ok: boolean; message: string }> {
  return fetchApi<{ ok: boolean; message: string }>('/admin/config/test', {
    method: 'POST',
    body: JSON.stringify(params),
  });
}

export interface ModelUsage {
  model: string;
  calls: number;
  prompt_tokens: number;
  completion_tokens: number;
}

export interface UserUsage {
  user_id: string;
  email?: string;
  calls: number;
  prompt_tokens: number;
  completion_tokens: number;
}

export interface AdminStats {
  total_calls: number;
  total_prompt_tokens: number;
  total_completion_tokens: number;
  by_model: ModelUsage[];
  by_user: UserUsage[];
}

export async function getAdminStats(days?: number): Promise<AdminStats> {
  const url = days ? `/admin/stats?days=${days}` : '/admin/stats';
  return fetchApi<AdminStats>(url);
}

export async function adminRechargeUser(userId: number, amount: number): Promise<void> {
  await fetchApi(`/admin/energy/users/${userId}/recharge`, {
    method: 'POST',
    body: JSON.stringify({ amount }),
  });
}

export async function getInviteCode(): Promise<{ code: string }> {
  return fetchApi<{ code: string }>('/energy/invite', { method: 'POST' });
}

export async function useInviteCode(code: string): Promise<{ status: string; reward: number }> {
  return fetchApi<{ status: string; reward: number }>('/energy/invite/use', {
    method: 'POST',
    body: JSON.stringify({ code }),
  });
}

export async function getPaymentPlans(): Promise<PaymentPlan[]> {
  const res = await fetch(`${API_BASE}/api/payment/plans`);
  const data = await res.json();
  return Array.isArray(data) ? data : [];
}

export async function createPaymentOrder(planId: string, channel: 'alipay' | 'wechat'): Promise<{ out_trade_no: string; pay_url?: string; code_url?: string }> {
  return fetchApi<{ out_trade_no: string; pay_url?: string; code_url?: string }>('/api/payment/order', {
    method: 'POST',
    body: JSON.stringify({ plan_id: planId, channel }),
  });
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

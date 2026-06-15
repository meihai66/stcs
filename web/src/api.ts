import type {
  Config,
  Favorite,
  GlobalSettings,
  HistoryItem,
  MarketItem,
  Profile,
  RequestLog,
  RequestLogMeta,
  StressStatus,
  Task,
  User,
} from './types'

async function req<T>(url: string, opts?: RequestInit): Promise<T> {
  const res = await fetch(url, opts)
  const text = await res.text()
  const data = text ? JSON.parse(text) : {}
  if (!res.ok) {
    throw new Error(data?.error?.message || `请求失败 (${res.status})`)
  }
  return data as T
}

function jsonBody(body: unknown): RequestInit {
  return {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

export const api = {
  // ---- 认证 ----
  authStatus: () => req<{ authed: boolean; required: boolean; user?: User }>('/api/auth/status'),
  captcha: () => req<{ id: string; image: string }>('/api/captcha'),
  login: (username: string, password: string, captchaId: string, captcha: string) =>
    req<{ ok: boolean; user: User }>(
      '/api/login',
      jsonBody({ username, password, captcha_id: captchaId, captcha }),
    ),
  logout: () => req<{ ok: boolean }>('/api/logout', { method: 'POST' }),
  changePassword: (oldPassword: string, newPassword: string) =>
    req<{ ok: boolean }>('/api/password', jsonBody({ old_password: oldPassword, new_password: newPassword })),

  // ---- 用户管理 / 全局设置(管理员) ----
  listUsers: () => req<{ users: User[] }>('/api/users'),
  createUser: (body: { username: string; password: string; role: string; image_limit: number }) =>
    req<{ ok: boolean; user: User }>('/api/users', jsonBody(body)),
  updateUser: (id: number, body: Record<string, unknown>) =>
    req<{ ok: boolean; user: User }>(`/api/users/${id}`, jsonBody(body)),
  deleteUser: (id: number) => req<{ ok: boolean }>(`/api/users/${id}`, { method: 'DELETE' }),
  getSettings: () => req<GlobalSettings>('/api/settings'),
  setSettings: (body: Record<string, unknown>) => req<{ ok: boolean }>('/api/settings', jsonBody(body)),

  // ---- 配置 / profile ----
  getConfig: () => req<Config>('/api/config'),
  saveConfig: (body: Record<string, unknown>) => req<{ ok: boolean }>('/api/config', jsonBody(body)),
  listProfiles: () => req<{ profiles: Profile[]; active: string }>('/api/profiles'),
  saveProfile: (body: Partial<Profile>) => req<{ ok: boolean }>('/api/profiles', jsonBody(body)),
  activateProfile: (name: string) =>
    req<{ ok: boolean }>('/api/profiles/activate', jsonBody({ name })),
  deleteProfile: (name: string) =>
    req<{ ok: boolean }>(`/api/profiles/${encodeURIComponent(name)}`, { method: 'DELETE' }),

  // ---- 生成 ----
  generate: (body: Record<string, unknown>) =>
    req<{ tasks: { id: string; status: string }[] }>('/api/generate', jsonBody(body)),
  edit: (form: FormData) =>
    req<{ images: { filename: string; url: string }[] }>('/api/edit', { method: 'POST', body: form }),
  listTasks: (all = false) => req<{ tasks: Task[] }>(`/api/tasks${all ? '?all=true' : ''}`),
  clearTasks: (all = false) =>
    req<{ ok: boolean; removed: number }>(`/api/tasks${all ? '?all=true' : ''}`, { method: 'DELETE' }),
  reversePrompt: (form: FormData) =>
    req<{ prompt: string }>('/api/reverse-prompt', { method: 'POST', body: form }),

  // ---- 历史 / 收藏 ----
  listHistory: (all = false) => req<{ history: HistoryItem[] }>(`/api/history${all ? '?all=true' : ''}`),
  deleteHistory: (id: number) => req<{ ok: boolean }>(`/api/history/${id}`, { method: 'DELETE' }),
  clearHistory: () => req<{ ok: boolean }>('/api/history', { method: 'DELETE' }),
  listFavorites: () => req<{ favorites: Favorite[] }>('/api/favorites'),
  addFavorite: (prompt: string, name: string) =>
    req<{ ok: boolean }>('/api/favorites', jsonBody({ prompt, name })),
  deleteFavorite: (id: number) => req<{ ok: boolean }>(`/api/favorites/${id}`, { method: 'DELETE' }),

  // ---- 市场 ----
  promptMarket: () => req<{ prompts: MarketItem[] }>('/api/prompt-market'),

  // ---- 压测 ----
  stressStart: (body: Record<string, unknown>) =>
    req<{ ok: boolean; capped: boolean; requested: number; cap: number }>(
      '/api/stress/start',
      jsonBody(body),
    ),
  stressStatus: () => req<StressStatus>('/api/stress/status'),
  stressStop: () => req<{ ok: boolean }>('/api/stress/stop', { method: 'POST' }),

  // ---- 请求日志(200 无图) ----
  requestLogs: (all = false) =>
    req<{ logs: RequestLogMeta[] }>(`/api/request-logs${all ? '?all=true' : ''}`),
  requestLog: (id: number) => req<{ log: RequestLog }>(`/api/request-logs/${id}`),
  clearRequestLogs: (all = false) =>
    req<{ ok: boolean }>(`/api/request-logs${all ? '?all=true' : ''}`, { method: 'DELETE' }),
}

export const SIZE_MATRIX: Record<string, Record<string, string>> = {
  '1:1': { '1K': '1024x1024', '2K': '2048x2048', '4K': '4096x4096' },
  '3:2': { '1K': '1536x1024', '2K': '3072x2048', '4K': '4096x2730' },
  '2:3': { '1K': '1024x1536', '2K': '2048x3072', '4K': '2730x4096' },
  '16:9': { '1K': '1344x768', '2K': '2560x1440', '4K': '3840x2160' },
  '9:16': { '1K': '768x1344', '2K': '1440x2560', '4K': '2160x3840' },
}

export const MODEL_PRESETS = [
  'gpt-image-2',
  'gpt-image-1',
  'gemini-2.5-flash-image',
  'gemini-2.5-flash-image-preview',
  'nano-banana',
]

export interface Config {
  base_url: string
  model: string
  reverse_model: string
  request_format: string
  default_size: string
  default_quality: string
  timeout: number
  concurrency: number
  api_key: string
  has_api_key: boolean
  server_api_key: string
  has_server_api_key: boolean
  running_workers: number
  active_profile: string
  username: string
  role: string
  image_limit: number
}

export interface User {
  id: number
  username: string
  role: string
  image_limit: number
  created_at: number
}

export interface GlobalSettings {
  log_limit: number
  default_image_limit: number
  concurrency: number
  has_server_api_key: boolean
}

export interface Profile {
  name: string
  base_url: string
  api_key: string
  has_api_key: boolean
  model: string
  reverse_model: string
  request_format: string
}

export interface ImageRef {
  filename: string
  url: string
}

export interface Task {
  id: string
  user_id?: number
  username?: string
  mode: string
  status: 'queued' | 'running' | 'done' | 'error'
  prompt: string
  size: string
  quality: string
  n: number
  model: string
  request_format: string
  created_at: number
  started_at: number | null
  finished_at: number | null
  images: ImageRef[]
  error: string
}

export interface HistoryItem {
  id: number
  user_id?: number
  username?: string
  created_at: number
  mode: string
  prompt: string
  model: string
  size: string
  quality: string
  n: number
  files: string[]
  images: ImageRef[]
}

export interface Favorite {
  id: number
  name: string
  prompt: string
  created_at: number
}

export interface MarketItem {
  title: string
  prompt: string
  preview: string
  author: string
  mode: string
  category: string
  reference_image_urls: string[]
}

export interface StressStatus {
  status: 'idle' | 'running' | 'done' | 'cancelled' | 'error'
  total: number
  concurrency: number
  model: string
  size: string
  fmt: string
  done: number
  ok: number
  fail: number
  elapsed: number
  rps: number
  lat_min: number
  lat_max: number
  lat_avg: number
  lat_p50: number
  lat_p95: number
  errors: Record<string, number>
  images?: ImageRef[]
  error?: string
}

export type Mode = 'images' | 'chat' | 'edit'

// ---- 画质 / 超分检测 ----
export interface InspectFinding {
  level: 'info' | 'warn' | 'bad'
  text: string
}
export interface InspectChannel {
  cutoff: number | null
  eff_px: number | null
  noise: number | null
}
export interface InspectSpectrum {
  centers: number[]
  whitened: number[]
  cutoff: number
}
export interface InspectClaim {
  claim: string
  pixel_ok: boolean
  eff_ok: boolean
  real: boolean
}
export interface InspectResult {
  file: string
  format: string
  width: number
  height: number
  filesize: number
  megapixels: number
  tier: string
  aspect: number
  preset: string
  preset_desc: string
  auto_note: string
  verdict: 'OK' | 'WARN' | 'FAIL'
  verdict_text: string
  verdict_color: string
  effective_resolution_px: number | null
  is_native: boolean | null
  ai_super_resolution_suspect: boolean
  interpolation: string
  effective: {
    cutoff: number
    upscale_factor: number
    upscale_factor_rounded: number
    hf_ratio: number
    plateau: number
    tail_drop_db: number
  } | null
  spectrum: InspectSpectrum | null
  channels: Record<'R' | 'G' | 'B', InspectChannel>
  chroma: {
    luma_cutoff: number | null
    chroma_cutoff: number | null
    chroma_luma_ratio: number | null
    subsample: string
  }
  chromatic_aberration: { shift_x: number; shift_y: number; magnitude: number; present: boolean }
  nearest_neighbor: { equal_fraction: number; is_nn: boolean; factor: number | null; block_consistency: number }
  noise_sigma: number | null
  sharpen: { overshoot_ratio: number; sharpened: boolean }
  metadata: {
    camera: string
    software: string
    software_hits: string[]
    datetime: string
    jpeg_quality: number | null
  }
  processing_chain: string[]
  reasons: string[]
  claim_check: InspectClaim | null
  findings: InspectFinding[]
}
export interface InspectItem {
  name: string
  ok: boolean
  error?: string
  result?: InspectResult
}

// 列表用:不含请求体/响应体
export interface RequestLogMeta {
  id: number
  time: number
  user_id?: number
  username?: string
  source: string
  endpoint: string
  model: string
  status: number
  reason: string
}

// 单条查询:含完整请求体/响应体
export interface RequestLog extends RequestLogMeta {
  request: string
  response: string
}

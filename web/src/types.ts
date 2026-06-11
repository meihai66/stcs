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

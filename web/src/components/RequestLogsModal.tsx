import { useCallback, useEffect, useState } from 'react'
import { RefreshCw, Trash2 } from 'lucide-react'
import { api } from '../api'
import type { RequestLog, RequestLogMeta } from '../types'
import Modal from './Modal'

interface Props {
  open: boolean
  onClose: () => void
  isAdmin?: boolean
}

const SOURCE_LABEL: Record<string, { text: string; cls: string }> = {
  images: { text: '文生图', cls: 'bg-brand-500' },
  chat: { text: '对话生图', cls: 'bg-violet-500' },
  edit: { text: '图生图', cls: 'bg-amber-500' },
  stress: { text: '压测', cls: 'bg-rose-500' },
}

// pretty 尽量把 JSON 美化输出,失败则原样返回。
function pretty(s: string): string {
  if (!s) return ''
  try {
    return JSON.stringify(JSON.parse(s), null, 2)
  } catch {
    return s
  }
}

function fmtTime(sec: number): string {
  const d = new Date(sec * 1000)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

export default function RequestLogsModal({ open, onClose, isAdmin }: Props) {
  const [logs, setLogs] = useState<RequestLogMeta[]>([])
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState('')
  const [filter, setFilter] = useState('')
  const [viewAll, setViewAll] = useState(false)
  // 展开的那一条 + 单条详情缓存(含请求体/响应体),点开才按 id 拉取
  const [openId, setOpenId] = useState<number | null>(null)
  const [detail, setDetail] = useState<Record<number, RequestLog>>({})
  const [detailErr, setDetailErr] = useState<Record<number, string>>({})
  const [detailLoading, setDetailLoading] = useState<number | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setErr('')
    try {
      setLogs((await api.requestLogs(isAdmin && viewAll)).logs || [])
      setDetail({})
      setDetailErr({})
      setOpenId(null)
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setLoading(false)
    }
  }, [isAdmin, viewAll])

  useEffect(() => {
    if (open) load()
  }, [open, load])

  async function toggle(id: number) {
    if (openId === id) {
      setOpenId(null)
      return
    }
    setOpenId(id)
    if (detail[id]) return // 已缓存
    setDetailLoading(id)
    setDetailErr((m) => ({ ...m, [id]: '' }))
    try {
      const { log } = await api.requestLog(id)
      setDetail((d) => ({ ...d, [id]: log }))
    } catch (e) {
      setDetailErr((m) => ({ ...m, [id]: (e as Error).message }))
    } finally {
      setDetailLoading(null)
    }
  }

  async function clear() {
    if (!window.confirm('确定清空请求日志?')) return
    await api.clearRequestLogs(isAdmin && viewAll)
    load()
  }

  const list = filter ? logs.filter((l) => l.source === filter) : logs

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="📋 请求日志"
      subtitle="记录「HTTP 200 但没拿到图片」的请求。列表只含概要,点任意一条才拉取并展示完整请求体与响应体。只存内存最近 1000 条,重启清空。"
      width={1000}
    >
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <select className="field !w-auto min-w-[120px]" value={filter} onChange={(e) => setFilter(e.target.value)}>
          <option value="">全部来源</option>
          <option value="images">文生图</option>
          <option value="chat">对话生图</option>
          <option value="edit">图生图</option>
          <option value="stress">压测</option>
        </select>
        <span className="text-xs text-slate-500">共 {list.length} 条</span>
        {isAdmin && (
          <button
            className={`btn btn-ghost !py-1.5 text-xs ${viewAll ? '!border-brand-500 !text-brand-400' : ''}`}
            onClick={() => setViewAll((v) => !v)}
          >
            {viewAll ? '全部用户' : '仅我'}
          </button>
        )}
        <div className="ml-auto flex gap-2">
          <button className="btn btn-ghost !py-1.5 text-xs" onClick={load}>
            <RefreshCw size={13} /> 刷新
          </button>
          <button className="btn btn-ghost !py-1.5 text-xs text-rose-400" onClick={clear} disabled={!logs.length}>
            <Trash2 size={13} /> 清空
          </button>
        </div>
      </div>

      {loading && <div className="card text-sm text-slate-500">加载中…</div>}
      {err && <div className="card text-sm text-rose-400">加载失败:{err}</div>}
      {!loading && !err && !list.length && (
        <div className="card text-sm text-slate-500">暂无记录 —— 说明每次 200 都正常拿到了图片 🎉</div>
      )}

      {!loading && !err && (
        <div className="flex flex-col gap-2">
          {list.map((l) => {
            const sl = SOURCE_LABEL[l.source] || { text: l.source, cls: 'bg-slate-500' }
            const expanded = openId === l.id
            const d = detail[l.id]
            return (
              <div key={l.id} className="overflow-hidden rounded-xl border border-[var(--color-line)] bg-[var(--color-ink-800)]">
                <button
                  className="flex w-full items-center gap-2.5 px-3.5 py-2.5 text-left hover:bg-slate-400/5"
                  onClick={() => toggle(l.id)}
                >
                  <span className={`shrink-0 rounded-full px-1.5 py-0.5 text-[10px] text-white ${sl.cls}`}>{sl.text}</span>
                  <span className="shrink-0 font-mono text-[11px] text-slate-500">{fmtTime(l.time)}</span>
                  <span className="shrink-0 rounded bg-emerald-500/15 px-1.5 py-0.5 text-[10px] font-medium text-emerald-400">
                    {l.status}
                  </span>
                  {l.username && (
                    <span className="shrink-0 rounded bg-brand-500/20 px-1.5 py-0.5 text-[10px] text-brand-400">{l.username}</span>
                  )}
                  <span className="truncate text-[13px] text-slate-300">{l.reason}</span>
                  <span className="ml-auto shrink-0 truncate text-[11px] text-slate-500">{l.model}</span>
                </button>
                {expanded && (
                  <div className="border-t border-[var(--color-line)] px-3.5 py-3">
                    <div className="mb-2 break-all text-[11px] text-slate-500">{l.endpoint}</div>
                    {detailLoading === l.id && <div className="text-xs text-slate-500">加载详情…</div>}
                    {detailErr[l.id] && <div className="text-xs text-rose-400">加载失败:{detailErr[l.id]}</div>}
                    {d && (
                      <>
                        <div className="mb-1 text-xs font-semibold text-slate-400">请求体</div>
                        <pre className="mb-3 max-h-60 overflow-auto rounded-lg bg-[var(--color-ink-900)] p-3 text-[11px] leading-relaxed text-slate-300">
                          {pretty(d.request) || '(空)'}
                        </pre>
                        <div className="mb-1 text-xs font-semibold text-slate-400">响应体</div>
                        <pre className="max-h-96 overflow-auto rounded-lg bg-[var(--color-ink-900)] p-3 text-[11px] leading-relaxed text-slate-300">
                          {pretty(d.response) || '(空)'}
                        </pre>
                      </>
                    )}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </Modal>
  )
}

import { useState } from 'react'
import { ChevronLeft, ChevronRight, Eye, Loader2, RefreshCw, Repeat, Trash2, Wand2 } from 'lucide-react'
import type { HistoryItem, Task } from '../types'
import Modal from './Modal'

const ST: Record<string, { label: string; cls: string }> = {
  queued: { label: '排队中', cls: 'bg-slate-700/40 text-slate-300' },
  running: { label: '生成中', cls: 'bg-brand-500/20 text-brand-400' },
  done: { label: '完成', cls: 'bg-emerald-500/15 text-emerald-400' },
  error: { label: '失败', cls: 'bg-rose-500/15 text-rose-400' },
}

// 每条历史最多内联展示的缩略图数量,超出折叠进「查看详情」。
const THUMB_CAP = 3

interface Props {
  tasks: Task[]
  history: HistoryItem[]
  historyTotal: number
  historyPage: number
  historyPageSize: number
  onHistoryPage: (page: number) => void
  showOwner?: boolean
  onLightbox: (url: string) => void
  onContinue: (filename: string) => void
  onReuse: (prompt: string) => void
  onDeleteHistory: (id: number) => void
  onRefreshTasks: () => void
  onClearTasks: () => void
  onClearHistory: () => void
}

export default function Workspace(p: Props) {
  const [detail, setDetail] = useState<HistoryItem | null>(null)
  const totalPages = Math.max(1, Math.ceil(p.historyTotal / p.historyPageSize))

  return (
    <main className="flex-1 overflow-y-auto p-7">
      {/* 任务队列 */}
      <div className="mb-4 flex items-center gap-3">
        <div className="sec-title flex-1">
          任务队列<span className="ln" />
        </div>
        <button className="btn btn-ghost !px-2.5 !py-1 text-xs" onClick={p.onRefreshTasks}>
          <RefreshCw size={13} /> 刷新
        </button>
        {!p.showOwner && p.tasks.some((t) => t.status === 'done' || t.status === 'error') && (
          <button
            className="btn btn-ghost !px-2.5 !py-1 text-xs hover:!border-rose-500 hover:!text-rose-400"
            onClick={() => {
              if (window.confirm('清理我所有已完成/失败的任务?(排队中、生成中的保留)')) p.onClearTasks()
            }}
          >
            <Trash2 size={13} /> 清理已完成
          </button>
        )}
      </div>
      {p.tasks.length === 0 ? (
        <div className="card text-sm text-slate-500">暂无任务</div>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {p.tasks.map((t) => (
            <TaskCard key={t.id} t={t} showOwner={p.showOwner} onLightbox={p.onLightbox} />
          ))}
        </div>
      )}

      {/* 历史 */}
      <div className="mb-4 mt-9 flex items-center gap-3">
        <div className="sec-title flex-1">
          历史记录
          {p.historyTotal > 0 && <span className="ml-2 text-xs font-normal text-slate-500">共 {p.historyTotal} 条</span>}
          <span className="ln" />
        </div>
        {!p.showOwner && p.history.length > 0 && (
          <button
            className="btn btn-ghost !px-2.5 !py-1 text-xs hover:!border-rose-500 hover:!text-rose-400"
            onClick={() => {
              if (window.confirm('清空我的全部历史记录?(已生成的图片文件保留)')) p.onClearHistory()
            }}
          >
            <Trash2 size={13} /> 清空历史
          </button>
        )}
      </div>
      {p.history.length === 0 ? (
        <div className="card text-sm text-slate-500">还没有生成记录</div>
      ) : (
        <div className="grid gap-3">
          {p.history.map((h) => {
            const shown = h.images.slice(0, THUMB_CAP)
            const extra = h.images.length - shown.length
            return (
              <div key={h.id} className="card flex gap-3.5">
                {h.images.length > 0 && (
                  <div className="flex max-w-[184px] shrink-0 flex-wrap content-start gap-2">
                    {shown.map((im) => (
                      <img
                        key={im.filename}
                        src={im.url}
                        loading="lazy"
                        decoding="async"
                        onClick={() => p.onLightbox(im.url)}
                        className="h-[88px] w-[88px] cursor-pointer rounded-lg border border-[var(--color-line)] object-cover transition hover:brightness-110"
                      />
                    ))}
                    {extra > 0 && (
                      <button
                        onClick={() => setDetail(h)}
                        title="查看全部图片"
                        className="grid h-[88px] w-[88px] place-items-center rounded-lg border border-[var(--color-line)] bg-[var(--color-ink-800)] text-sm font-medium text-slate-300 transition hover:border-brand-500 hover:text-brand-400"
                      >
                        +{extra}
                      </button>
                    )}
                  </div>
                )}
                <div className="flex min-w-0 flex-1 flex-col">
                  <div
                    className="line-clamp-3 break-words text-sm font-medium leading-relaxed text-slate-200"
                    title={h.prompt}
                  >
                    {h.prompt || '(无提示词)'}
                  </div>
                  <div className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-slate-500">
                    {p.showOwner && h.username && (
                      <span className="rounded bg-brand-500/20 px-1.5 py-0.5 text-brand-400">{h.username}</span>
                    )}
                    <span>{new Date(h.created_at * 1000).toLocaleString('zh-CN')}</span>
                    {[h.mode, h.size, h.quality, h.model].filter(Boolean).map((v, i) => (
                      <span key={i} className="break-all rounded bg-[var(--color-ink-800)] px-1.5 py-0.5">
                        {v}
                      </span>
                    ))}
                  </div>
                  <div className="mt-2.5 flex flex-wrap gap-1.5">
                    {h.images.length > 0 && (
                      <button className="btn btn-ghost !px-2.5 !py-1 text-xs" onClick={() => setDetail(h)}>
                        <Eye size={12} /> 查看详情
                      </button>
                    )}
                    <button className="btn btn-ghost !px-2.5 !py-1 text-xs" onClick={() => p.onReuse(h.prompt)}>
                      <Wand2 size={12} /> 再用此提示词
                    </button>
                    {h.files[0] && (
                      <button
                        className="btn btn-ghost !px-2.5 !py-1 text-xs"
                        onClick={() => p.onContinue(h.files[0])}
                      >
                        <Repeat size={12} /> 在此基础上继续
                      </button>
                    )}
                    <button
                      className="btn btn-ghost !px-2.5 !py-1 text-xs hover:!border-rose-500 hover:!text-rose-400"
                      onClick={() => p.onDeleteHistory(h.id)}
                    >
                      <Trash2 size={12} /> 删除记录
                    </button>
                  </div>
                </div>
              </div>
            )
          })}
        </div>
      )}

      {totalPages > 1 && (
        <Pager page={p.historyPage} totalPages={totalPages} onPage={p.onHistoryPage} />
      )}

      {/* 单条历史详情 */}
      <Modal
        open={!!detail}
        onClose={() => setDetail(null)}
        width={760}
        title="生成详情"
        subtitle={detail ? new Date(detail.created_at * 1000).toLocaleString('zh-CN') : ''}
      >
        {detail && (
          <div className="flex flex-col gap-4">
            <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-slate-500">
              {p.showOwner && detail.username && (
                <span className="rounded bg-brand-500/20 px-1.5 py-0.5 text-brand-400">{detail.username}</span>
              )}
              <span className="rounded bg-[var(--color-ink-800)] px-1.5 py-0.5">{detail.images.length} 张</span>
              {[detail.mode, detail.size, detail.quality, detail.model].filter(Boolean).map((v, i) => (
                <span key={i} className="break-all rounded bg-[var(--color-ink-800)] px-1.5 py-0.5">
                  {v}
                </span>
              ))}
            </div>
            <div className="whitespace-pre-wrap break-words text-sm leading-relaxed text-slate-200">
              {detail.prompt || '(无提示词)'}
            </div>
            <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-3">
              {detail.images.map((im) => (
                <img
                  key={im.filename}
                  src={im.url}
                  loading="lazy"
                  decoding="async"
                  onClick={() => p.onLightbox(im.url)}
                  className="aspect-square w-full cursor-pointer rounded-lg border border-[var(--color-line)] object-cover transition hover:brightness-110"
                />
              ))}
            </div>
          </div>
        )}
      </Modal>
    </main>
  )
}

// Pager 紧凑分页器:首页 … 当前页附近 … 末页。
function Pager({ page, totalPages, onPage }: { page: number; totalPages: number; onPage: (p: number) => void }) {
  const nums: (number | string)[] = []
  const push = (n: number) => nums.push(n)
  push(1)
  const from = Math.max(2, page - 1)
  const to = Math.min(totalPages - 1, page + 1)
  if (from > 2) nums.push('…l')
  for (let i = from; i <= to; i++) push(i)
  if (to < totalPages - 1) nums.push('…r')
  if (totalPages > 1) push(totalPages)

  return (
    <div className="mt-6 flex flex-wrap items-center justify-center gap-1.5 text-sm">
      <button
        className="btn btn-ghost !px-2 !py-1 text-xs disabled:opacity-40"
        disabled={page <= 1}
        onClick={() => onPage(page - 1)}
        title="上一页"
      >
        <ChevronLeft size={14} />
      </button>
      {nums.map((n) =>
        typeof n === 'string' ? (
          <span key={n} className="px-1.5 text-slate-600">
            …
          </span>
        ) : (
          <button
            key={n}
            onClick={() => onPage(n)}
            className={`min-w-[32px] rounded-lg px-2 py-1 text-xs font-medium transition ${
              n === page
                ? 'bg-brand-500/20 text-brand-400'
                : 'text-slate-400 hover:bg-slate-400/10 hover:text-slate-100'
            }`}
          >
            {n}
          </button>
        ),
      )}
      <button
        className="btn btn-ghost !px-2 !py-1 text-xs disabled:opacity-40"
        disabled={page >= totalPages}
        onClick={() => onPage(page + 1)}
        title="下一页"
      >
        <ChevronRight size={14} />
      </button>
    </div>
  )
}

function TaskCard({ t, showOwner, onLightbox }: { t: Task; showOwner?: boolean; onLightbox: (u: string) => void }) {
  const st = ST[t.status] || { label: t.status, cls: 'bg-slate-700/40 text-slate-300' }
  const elapsed =
    t.status === 'running' && t.started_at
      ? ' ' + Math.max(0, Math.round(Date.now() / 1000 - t.started_at)) + 's'
      : ''
  return (
    <div className="card">
      <div className="flex items-center gap-2">
        <div className="min-w-0 flex-1 truncate text-[13px] font-medium text-slate-200" title={t.prompt}>
          {t.prompt}
        </div>
        <span className={`flex items-center gap-1 whitespace-nowrap rounded-full px-2.5 py-1 text-[11px] font-medium ${st.cls}`}>
          {t.status === 'running' && <Loader2 size={11} className="spin" />}
          {st.label}
          {elapsed}
        </span>
      </div>
      <div className="mt-1.5 text-[11px] text-slate-500">
        {showOwner && t.username && (
          <span className="mr-1.5 rounded bg-brand-500/20 px-1.5 py-0.5 text-brand-400">{t.username}</span>
        )}
        {t.request_format === 'chat' ? 'chat' : 'images'} · {t.size} · {t.quality} · n={t.n} · {t.model}
      </div>
      {t.error && (
        <div className="mt-1.5 whitespace-pre-wrap text-[11px] leading-relaxed text-rose-400">{t.error}</div>
      )}
      {!!t.images.length && (
        <div className="mt-2 flex flex-wrap gap-2">
          {t.images.map((im) => (
            <img
              key={im.filename}
              src={im.url}
              loading="lazy"
              decoding="async"
              onClick={() => onLightbox(im.url)}
              className="h-[72px] w-[72px] cursor-pointer rounded-lg border border-[var(--color-line)] object-cover transition hover:brightness-110"
            />
          ))}
        </div>
      )}
    </div>
  )
}

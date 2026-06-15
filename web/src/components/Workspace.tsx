import { Loader2, RefreshCw, Trash2, Repeat, Wand2 } from 'lucide-react'
import type { HistoryItem, Task } from '../types'

const ST: Record<string, { label: string; cls: string }> = {
  queued: { label: '排队中', cls: 'bg-slate-700/40 text-slate-300' },
  running: { label: '生成中', cls: 'bg-brand-500/20 text-brand-400' },
  done: { label: '完成', cls: 'bg-emerald-500/15 text-emerald-400' },
  error: { label: '失败', cls: 'bg-rose-500/15 text-rose-400' },
}

interface Props {
  tasks: Task[]
  history: HistoryItem[]
  onLightbox: (url: string) => void
  onContinue: (filename: string) => void
  onReuse: (prompt: string) => void
  onDeleteHistory: (id: number) => void
  onRefreshTasks: () => void
  onClearTasks: () => void
  onClearHistory: () => void
}

export default function Workspace(p: Props) {
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
        {p.tasks.some((t) => t.status === 'done' || t.status === 'error') && (
          <button
            className="btn btn-ghost !px-2.5 !py-1 text-xs hover:!border-rose-500 hover:!text-rose-400"
            onClick={() => {
              if (window.confirm('清理所有已完成/失败的任务?(排队中、生成中的保留)')) p.onClearTasks()
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
            <TaskCard key={t.id} t={t} onLightbox={p.onLightbox} />
          ))}
        </div>
      )}

      {/* 历史 */}
      <div className="mb-4 mt-9 flex items-center gap-3">
        <div className="sec-title flex-1">
          历史记录<span className="ln" />
        </div>
        {p.history.length > 0 && (
          <button
            className="btn btn-ghost !px-2.5 !py-1 text-xs hover:!border-rose-500 hover:!text-rose-400"
            onClick={() => {
              if (window.confirm('清空全部历史记录?(已生成的图片文件保留)')) p.onClearHistory()
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
          {p.history.map((h) => (
            <div key={h.id} className="card flex gap-3.5">
              <div className="flex flex-wrap gap-2">
                {h.images.map((im) => (
                  <img
                    key={im.filename}
                    src={im.url}
                    onClick={() => p.onLightbox(im.url)}
                    className="h-[88px] w-[88px] cursor-pointer rounded-lg border border-[var(--color-line)] object-cover transition hover:brightness-110"
                  />
                ))}
              </div>
              <div className="min-w-0 flex-1">
                <div className="break-words text-sm font-medium text-slate-200">{h.prompt}</div>
                <div className="mt-1 text-[11px] text-slate-500">
                  {new Date(h.created_at * 1000).toLocaleString('zh-CN')} · {h.mode} · {h.size} · {h.quality} · {h.model}
                </div>
                <div className="mt-2.5 flex flex-wrap gap-1.5">
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
          ))}
        </div>
      )}
    </main>
  )
}

function TaskCard({ t, onLightbox }: { t: Task; onLightbox: (u: string) => void }) {
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
              onClick={() => onLightbox(im.url)}
              className="h-[72px] w-[72px] cursor-pointer rounded-lg border border-[var(--color-line)] object-cover transition hover:brightness-110"
            />
          ))}
        </div>
      )}
    </div>
  )
}

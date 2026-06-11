import { useEffect, useState } from 'react'
import { Play, Square, Zap } from 'lucide-react'
import { api } from '../api'
import type { Config, StressStatus } from '../types'
import Modal from './Modal'
import SizePicker, { sizeOf, type SizeState } from './SizePicker'

interface Props {
  open: boolean
  onClose: () => void
  config: Config
  onLightbox: (url: string) => void
}

const STLABEL: Record<string, string> = {
  running: '运行中',
  done: '已完成',
  cancelled: '已停止',
  error: '出错',
}

export default function StressModal({ open, onClose, config, onLightbox }: Props) {
  const [prompt, setPrompt] = useState('a cute cat')
  const [total, setTotal] = useState(20)
  const [conc, setConc] = useState(5)
  const [model, setModel] = useState('')
  const [fmt, setFmt] = useState('images')
  const [quality, setQuality] = useState('high')
  const [save, setSave] = useState(true)
  const [size, setSize] = useState<SizeState>({ ratio: '1:1', tier: '1K', custom: '' })
  const [stat, setStat] = useState<StressStatus | null>(null)
  const [note, setNote] = useState('')

  const running = stat?.status === 'running'

  async function refresh() {
    try {
      setStat(await api.stressStatus())
    } catch {
      /* ignore */
    }
  }

  // 弹窗打开期间持续轮询(运行中 800ms 一次,空闲时也保持轮询以便
  // 关掉弹窗再打开时进度能继续刷新——旧版重开弹窗后轮询不恢复,看起来像卡死)
  useEffect(() => {
    if (!open) return
    setFmt(config.request_format === 'chat' ? 'chat' : 'images')
    refresh()
    const id = window.setInterval(() => {
      refresh()
    }, 800)
    return () => clearInterval(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  async function start() {
    setNote('')
    try {
      const r = await api.stressStart({
        prompt: prompt || 'a cute cat',
        total,
        concurrency: conc,
        model: model.trim() || null,
        size: sizeOf(size),
        quality,
        request_format: fmt,
        save,
      })
      if (r.capped) setNote(`并发 ${r.requested} 超过上限,已限到 ${r.cap}`)
      refresh()
    } catch (e) {
      setNote((e as Error).message)
    }
  }

  async function stop() {
    await api.stressStop()
    refresh()
  }

  const pct = stat && stat.total ? Math.round((stat.done / stat.total) * 100) : 0
  const images = stat?.images || []

  return (
    <Modal open={open} onClose={onClose} title="⚡ 生图压测" width={640}>
      <div className="mb-3 rounded-xl border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-300">
        ⚠️ 压测会真实调用中转站、产生计费/消耗额度!并发无硬上限,但填太高会被中转站限流(429)或耗尽本机端口。建议从
        20/50/100/200 往上测,找失败率开始飙升的临界点。
      </div>

      <label className="lbl !mt-0">压测提示词</label>
      <input className="field" value={prompt} onChange={(e) => setPrompt(e.target.value)} />

      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="lbl">总请求数</label>
          <input className="field" type="number" min={1} value={total} onChange={(e) => setTotal(+e.target.value)} />
        </div>
        <div>
          <label className="lbl">并发数(无硬上限)</label>
          <input className="field" type="number" min={1} value={conc} onChange={(e) => setConc(+e.target.value)} />
        </div>
      </div>

      <div className="grid grid-cols-3 gap-3">
        <div>
          <label className="lbl">模型</label>
          <input className="field" value={model} onChange={(e) => setModel(e.target.value)} placeholder={`默认(${config.model || 'gpt-image-2'})`} />
        </div>
        <div>
          <label className="lbl">请求格式</label>
          <select className="field" value={fmt} onChange={(e) => setFmt(e.target.value)}>
            <option value="images">images</option>
            <option value="chat">chat</option>
          </select>
        </div>
        <div>
          <label className="lbl">质量</label>
          <select className="field" value={quality} onChange={(e) => setQuality(e.target.value)}>
            {['high', 'medium', 'low', 'auto'].map((q) => (
              <option key={q}>{q}</option>
            ))}
          </select>
        </div>
      </div>

      <div className="mt-1">
        <SizePicker value={size} onChange={setSize} />
      </div>

      <label className="mt-4 flex cursor-pointer items-center gap-2 text-sm text-slate-300">
        <input
          type="checkbox"
          checked={save}
          onChange={(e) => setSave(e.target.checked)}
          className="h-4 w-4 accent-indigo-500"
        />
        保存生成的图片(最多 200 张,写入历史记录;高并发时建议关闭以省内存/磁盘)
      </label>

      <div className="mt-4 flex gap-2">
        <button className="btn btn-primary flex-1 py-2.5" onClick={start} disabled={running}>
          <Play size={16} /> 开始压测
        </button>
        <button className="btn btn-ghost flex-1 py-2.5" onClick={stop} disabled={!running}>
          <Square size={15} /> 停止
        </button>
      </div>
      {note && <div className="mt-2 text-sm text-rose-400">{note}</div>}

      {stat && stat.status !== 'idle' && (
        <div className="mt-5">
          {stat.error ? (
            <div className="card text-sm text-rose-400">{stat.error}</div>
          ) : (
            <>
              <div className="mb-1.5 flex items-center justify-between text-sm">
                <b className="flex items-center gap-1.5 text-slate-200">
                  <Zap size={14} className="text-brand-400" />
                  {STLABEL[stat.status] || stat.status}
                </b>
                <span className="text-slate-400">
                  {stat.done}/{stat.total} ({pct}%)
                </span>
              </div>
              <div className="mb-4 h-2 overflow-hidden rounded-full bg-[var(--color-ink-800)]">
                <div
                  className="h-full bg-gradient-to-r from-brand-500 to-fuchsia-500 transition-all"
                  style={{ width: `${pct}%` }}
                />
              </div>
              <div className="grid grid-cols-4 gap-2.5">
                <Cell label="成功" value={stat.ok} color="text-emerald-400" />
                <Cell label="失败" value={stat.fail} color="text-rose-400" />
                <Cell label="吞吐 req/s" value={stat.rps} />
                <Cell label="用时 s" value={stat.elapsed} />
                <Cell label="平均 ms" value={stat.lat_avg} />
                <Cell label="P50 ms" value={stat.lat_p50} />
                <Cell label="P95 ms" value={stat.lat_p95} />
                <Cell label="最大 ms" value={stat.lat_max} />
              </div>
              <div className="mt-3.5 text-[11px] text-slate-500">
                并发 {stat.concurrency} · {stat.fmt} · {stat.size} · {stat.model}
              </div>
              <div className="mt-3">
                <div className="mb-2 sec-title">错误分类<span className="ln" /></div>
                {Object.keys(stat.errors || {}).length === 0 ? (
                  <div className="text-xs text-slate-500">无</div>
                ) : (
                  <div className="grid gap-1">
                    {Object.entries(stat.errors).map(([k, v]) => (
                      <div key={k} className="flex justify-between gap-3 text-xs">
                        <span className="truncate text-rose-400" title={k}>{k}</span>
                        <b className="shrink-0 text-slate-300">{v}</b>
                      </div>
                    ))}
                  </div>
                )}
              </div>
              {!!images.length && (
                <div className="mt-3">
                  <div className="mb-2 sec-title">
                    生成的图片({images.length})<span className="ln" />
                  </div>
                  <div className="flex flex-wrap gap-2">
                    {images.map((im) => (
                      <img
                        key={im.filename}
                        src={im.url}
                        loading="lazy"
                        onClick={() => onLightbox(im.url)}
                        className="h-[72px] w-[72px] cursor-pointer rounded-lg border border-[var(--color-line)] object-cover transition hover:brightness-110"
                      />
                    ))}
                  </div>
                  <div className="mt-2 text-[11px] text-slate-500">
                    点击图片放大查看,放大后可下载保存;压测结束后也可在「历史记录」中找到这批图。
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      )}
    </Modal>
  )
}

function Cell({ label, value, color }: { label: string; value: number; color?: string }) {
  return (
    <div className="rounded-xl border border-[var(--color-line)] bg-[var(--color-ink-800)] p-2.5 text-center">
      <div className={`text-lg font-bold ${color || 'text-slate-100'}`}>{value ?? 0}</div>
      <div className="mt-0.5 text-[10px] text-slate-500">{label}</div>
    </div>
  )
}

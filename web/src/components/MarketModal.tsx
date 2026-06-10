import { useEffect, useMemo, useState } from 'react'
import { api } from '../api'
import type { MarketItem, Mode } from '../types'
import Modal from './Modal'

interface Props {
  open: boolean
  onClose: () => void
  onApply: (prompt: string, mode: Mode) => void
  onLightbox: (url: string) => void
}

export default function MarketModal({ open, onClose, onApply, onLightbox }: Props) {
  const [data, setData] = useState<MarketItem[]>([])
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState('')
  const [q, setQ] = useState('')
  const [cat, setCat] = useState('')
  const [md, setMd] = useState('')

  useEffect(() => {
    if (!open || data.length) return
    setLoading(true)
    setErr('')
    api
      .promptMarket()
      .then((r) => setData(r.prompts || []))
      .catch((e) => setErr((e as Error).message))
      .finally(() => setLoading(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  const cats = useMemo(() => [...new Set(data.map((x) => x.category).filter(Boolean))], [data])
  const list = useMemo(() => {
    const kw = q.trim().toLowerCase()
    return data
      .filter(
        (x) =>
          (!cat || x.category === cat) &&
          (!md || x.mode === md) &&
          (!kw || (x.title + x.prompt + x.author + x.category).toLowerCase().includes(kw)),
      )
      .slice(0, 120)
  }, [data, q, cat, md])

  return (
    <Modal open={open} onClose={onClose} title="🛒 提示词市场" subtitle="数据来自 glidea/banana-prompt-quicker。点「套用」自动填入并切换模式。" width={1040}>
      <div className="mb-4 flex flex-wrap gap-2">
        <input className="field flex-[2] min-w-[180px]" placeholder="搜索标题 / 提示词 / 作者 / 分类" value={q} onChange={(e) => setQ(e.target.value)} />
        <select className="field flex-1 min-w-[120px]" value={cat} onChange={(e) => setCat(e.target.value)}>
          <option value="">全部分类</option>
          {cats.map((c) => (
            <option key={c} value={c}>{c}</option>
          ))}
        </select>
        <select className="field flex-1 min-w-[120px]" value={md} onChange={(e) => setMd(e.target.value)}>
          <option value="">全部模式</option>
          <option value="generate">文生图</option>
          <option value="edit">编辑</option>
        </select>
      </div>

      {loading && <div className="card text-sm text-slate-500">加载中…</div>}
      {err && <div className="card text-sm text-rose-400">加载失败:{err}</div>}
      {!loading && !err && (
        <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-3 lg:grid-cols-4">
          {list.map((x, i) => (
            <div key={i} className="flex flex-col overflow-hidden rounded-xl border border-[var(--color-line)] bg-[var(--color-ink-800)]">
              {x.preview && (
                <img
                  src={x.preview}
                  loading="lazy"
                  onClick={() => onLightbox(x.preview)}
                  className="h-36 w-full cursor-pointer bg-[var(--color-ink-900)] object-cover"
                />
              )}
              <div className="flex flex-1 flex-col gap-1.5 p-3">
                <div className="flex items-center gap-1.5 text-[13px] font-semibold text-slate-200">
                  <span className="truncate">{x.title || '(无标题)'}</span>
                  <span className={`shrink-0 rounded-full px-1.5 py-0.5 text-[10px] text-white ${x.mode === 'edit' ? 'bg-amber-500' : 'bg-brand-500'}`}>
                    {x.mode === 'edit' ? '编辑' : '文生图'}
                  </span>
                </div>
                <div className="text-[11px] text-slate-500">{x.author} · {x.category}</div>
                <div className="line-clamp-2 text-[11px] text-slate-500">{x.prompt}</div>
                <button
                  className="btn btn-primary mt-auto !py-1.5 text-xs"
                  onClick={() => { onApply(x.prompt, x.mode === 'edit' ? 'edit' : 'images'); onClose() }}
                >
                  套用
                </button>
              </div>
            </div>
          ))}
          {!list.length && <div className="col-span-full card text-sm text-slate-500">没有匹配的提示词</div>}
        </div>
      )}
    </Modal>
  )
}

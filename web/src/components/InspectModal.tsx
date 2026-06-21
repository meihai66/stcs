import { useRef, useState } from 'react'
import { ScanSearch, UploadCloud, X } from 'lucide-react'
import { api } from '../api'
import type { InspectItem, InspectSpectrum } from '../types'
import Modal from './Modal'

interface Props {
  open: boolean
  onClose: () => void
}

interface Card {
  item: InspectItem
  thumb: string // objectURL
}

const PRESETS = [
  ['auto', '自动识别'],
  ['photo', '照片'],
  ['anime', '动漫/插画'],
  ['game', '游戏截图'],
  ['screenshot', '截图/文档'],
]
const CLAIMS = [
  ['', '不核验'],
  ['8k', '8K'],
  ['4k', '4K'],
  ['2k', '2K'],
  ['1k', '1K(1080p)'],
  ['720p', '720p'],
]

export default function InspectModal({ open, onClose }: Props) {
  const [files, setFiles] = useState<File[]>([])
  const [preset, setPreset] = useState('auto')
  const [claim, setClaim] = useState('')
  const [loading, setLoading] = useState(false)
  const [cards, setCards] = useState<Card[]>([])
  const [err, setErr] = useState('')
  const [hot, setHot] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  function addFiles(list: FileList | null) {
    if (!list) return
    const imgs = Array.from(list).filter((f) => f.type.startsWith('image/') || /\.(jpe?g|png|webp|bmp|tiff?|gif)$/i.test(f.name))
    if (imgs.length) setFiles((prev) => [...prev, ...imgs])
  }

  async function run() {
    if (!files.length || loading) return
    setLoading(true)
    setErr('')
    const batch = files
    const thumbs = batch.map((f) => URL.createObjectURL(f))
    try {
      const form = new FormData()
      batch.forEach((f) => form.append('files', f))
      form.append('preset', preset)
      form.append('claim', claim)
      const { results } = await api.inspect(form)
      const newCards: Card[] = results.map((item, i) => ({ item, thumb: thumbs[i] || '' }))
      setCards((prev) => [...newCards, ...prev])
      setFiles([])
    } catch (e) {
      thumbs.forEach((u) => URL.revokeObjectURL(u))
      setErr((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  function clearAll() {
    cards.forEach((c) => c.thumb && URL.revokeObjectURL(c.thumb))
    setCards([])
    setFiles([])
    setErr('')
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      width={1000}
      title={
        <span className="flex items-center gap-2">
          <ScanSearch size={20} className="text-brand-400" /> 画质 · 分辨率 · 超分检测
        </span>
      }
      subtitle="拖拽或选择图片 → 判断是否真 4K/2K/1K、是否被放大、用何手段、画质是否人为提升。绿=原生真实 · 橙=画质经处理 · 红=疑似放大/重建。纯本地运算,图片不出本机。"
    >
      {/* 工具栏 */}
      <div className="flex flex-wrap items-end gap-3">
        <div>
          <label className="lbl !mt-0">内容类型预设</label>
          <select className="field !w-auto !py-2" value={preset} onChange={(e) => setPreset(e.target.value)}>
            {PRESETS.map(([v, t]) => (
              <option key={v} value={v}>{t}</option>
            ))}
          </select>
        </div>
        <div>
          <label className="lbl !mt-0">核验标称分级</label>
          <select className="field !w-auto !py-2" value={claim} onChange={(e) => setClaim(e.target.value)}>
            {CLAIMS.map(([v, t]) => (
              <option key={v} value={v}>{t}</option>
            ))}
          </select>
        </div>
        <button className="btn btn-primary py-2.5" onClick={run} disabled={!files.length || loading}>
          {loading ? '检测中…' : `开始检测${files.length ? ` (${files.length})` : ''}`}
        </button>
        {cards.length > 0 && (
          <button className="btn btn-ghost py-2.5" onClick={clearAll} disabled={loading}>
            清空结果
          </button>
        )}
      </div>

      {/* 拖拽区 */}
      <div
        className={`mt-3 cursor-pointer rounded-2xl border-2 border-dashed p-7 text-center transition-colors ${
          hot ? 'border-brand-500 bg-brand-500/10 text-brand-300' : 'border-[var(--color-line)] text-slate-400'
        }`}
        onClick={() => inputRef.current?.click()}
        onDragOver={(e) => { e.preventDefault(); setHot(true) }}
        onDragLeave={() => setHot(false)}
        onDrop={(e) => { e.preventDefault(); setHot(false); addFiles(e.dataTransfer.files) }}
      >
        <UploadCloud size={26} className="mx-auto mb-1.5 opacity-70" />
        把图片拖到这里,或<b className="text-brand-400"> 点击选择</b>
        <div className="mt-1 text-[11px] text-slate-500">支持 jpg/png/webp/bmp/tiff/gif · 可多选 · 单图 ≤ 40MB</div>
        <input
          ref={inputRef}
          type="file"
          multiple
          accept="image/*,.jpg,.jpeg,.png,.webp,.bmp,.tif,.tiff,.gif"
          className="hidden"
          onChange={(e) => { addFiles(e.target.files); e.target.value = '' }}
        />
      </div>

      {/* 待检测文件名 */}
      {files.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-1.5">
          {files.map((f, i) => (
            <span key={i} className="chip flex items-center gap-1 !py-1">
              {f.name}
              <X
                size={12}
                className="opacity-60 hover:opacity-100"
                onClick={(e) => { e.stopPropagation(); setFiles((p) => p.filter((_, k) => k !== i)) }}
              />
            </span>
          ))}
        </div>
      )}

      {err && <div className="mt-3 text-sm text-rose-400">{err}</div>}

      {/* 结果卡片 */}
      <div className="mt-4 grid gap-4">
        {cards.map((c, i) => (
          <ResultCard key={i} card={c} />
        ))}
      </div>
    </Modal>
  )
}

function ResultCard({ card }: { card: Card }) {
  const { item, thumb } = card
  if (!item.ok || !item.result) {
    return (
      <div className="card">
        <div className="flex items-center justify-between">
          <b className="text-slate-200">{item.name}</b>
          <span className="rounded-full bg-slate-500/30 px-3 py-1 text-xs text-slate-300">无法分析</span>
        </div>
        <div className="mt-2 text-sm text-rose-400">{item.error || '未知错误'}</div>
      </div>
    )
  }
  const r = item.result
  const cr = r.claim_check
  return (
    <div className="card">
      <div className="mb-3 flex items-center justify-between gap-3 border-b border-[var(--color-line)] pb-2.5">
        <b className="truncate text-[15px] text-slate-100">{item.name}</b>
        <span
          className="shrink-0 rounded-full px-3 py-1 text-[13px] font-semibold text-white"
          style={{ background: r.verdict_color }}
        >
          {r.verdict_text}
        </span>
      </div>

      <div className="flex flex-wrap gap-5">
        <div className="min-w-[280px] flex-1">
          <table className="w-full text-[13px]">
            <tbody>
              <Row k="检测预设" v={`${r.preset_desc}${r.auto_note ? ` (${r.auto_note})` : ''}`} />
              <Row k="格式/大小" v={`${r.format} · ${(r.filesize / 1024).toFixed(1)} KB`} />
              <Row k="分辨率" v={`${r.width}×${r.height} (${r.megapixels.toFixed(1)}MP)`} />
              <Row k="像素分级" v={<b className="text-slate-100">{r.tier}</b>} />
              <Row k="有效分辨率" v={r.effective_resolution_px ? `≈${r.effective_resolution_px.toFixed(0)}px` : 'n/a'} />
              {cr && (
                <Row
                  k="标称核验"
                  v={
                    <span style={{ color: cr.real ? '#22c55e' : '#ef4444', fontWeight: 600 }}>
                      声称 {cr.claim} → {cr.real ? '真 PASS' : '假 FAIL'}
                    </span>
                  }
                />
              )}
            </tbody>
          </table>
          {r.spectrum && <Spectrum spec={r.spectrum} />}
        </div>
        <div className="min-w-[200px] flex-1">
          {thumb && <img src={thumb} className="max-h-[220px] w-full rounded-lg border border-[var(--color-line)] object-contain" />}
        </div>
      </div>

      <Section title="检测明细" />
      <div className="grid gap-0.5 text-[13px]">
        {r.findings.map((f, i) => (
          <div key={i} className={findColor(f.level)}>
            {f.level === 'bad' ? '‼ ' : f.level === 'warn' ? '! ' : '· '}
            {f.text}
          </div>
        ))}
      </div>

      <Section title="色彩通道 / 色度 / 色差" />
      <table className="w-full text-[13px]">
        <tbody>
          {(['R', 'G', 'B'] as const).map((ch) => {
            const d = r.channels[ch]
            return (
              <Row
                key={ch}
                k={`${ch} 通道`}
                v={`有效≈${d.eff_px ? d.eff_px.toFixed(0) + 'px' : 'n/a'} / 截止 ${d.cutoff ? d.cutoff.toFixed(2) : 'n/a'} / σ≈${d.noise != null ? d.noise.toFixed(2) : 'n/a'}`}
              />
            )
          })}
          {r.chroma.chroma_luma_ratio != null && (
            <Row k="色度/亮度比" v={`${r.chroma.chroma_luma_ratio.toFixed(2)} — ${r.chroma.subsample || '—'}`} />
          )}
          <Row
            k="镜头色差 CA"
            v={`位移≈${r.chromatic_aberration.magnitude.toFixed(1)}px (${r.chromatic_aberration.present ? '有,符合真实镜头' : '无'})`}
          />
        </tbody>
      </table>

      <Section title="推断处理链" />
      <div className="rounded-lg bg-[var(--color-ink-800)] px-3 py-2 font-mono text-[12px] text-slate-300">
        {r.processing_chain.length ? `原图 → ${r.processing_chain.join(' → ')} → 当前图` : '未检出明显放大/增强(疑似原生)'}
      </div>

      <Section title="结论原因" />
      {r.reasons.length ? (
        <ul className="ml-4 list-disc text-[13px] text-slate-300">
          {r.reasons.map((x, i) => (
            <li key={i} className="my-0.5">{x}</li>
          ))}
        </ul>
      ) : (
        <div className="text-[13px] text-emerald-400">无(原生真实)</div>
      )}
    </div>
  )
}

function Row({ k, v }: { k: string; v: React.ReactNode }) {
  return (
    <tr>
      <td className="w-[96px] py-1 align-top text-slate-500">{k}</td>
      <td className="py-1 text-slate-200">{v}</td>
    </tr>
  )
}

function Section({ title }: { title: string }) {
  return (
    <div className="mb-1.5 mt-4 flex items-center gap-3 text-[11px] font-semibold uppercase tracking-wider text-slate-500">
      {title}
      <span className="h-px flex-1 bg-[var(--color-line)]" />
    </div>
  )
}

function findColor(level: string) {
  if (level === 'bad') return 'text-rose-400 font-semibold'
  if (level === 'warn') return 'text-amber-400'
  return 'text-slate-400'
}

function Spectrum({ spec }: { spec: InspectSpectrum }) {
  const w = 380, h = 140, pad = 26
  const X = (x: number) => pad + x * (w - 2 * pad)
  const Y = (y: number) => h - pad - y * (h - 2 * pad)
  const pts = spec.centers.map((c, i) => `${X(c).toFixed(1)},${Y(spec.whitened[i]).toFixed(1)}`).join(' ')
  const cutX = X(spec.cutoff)
  const halfX = X(0.5)
  return (
    <svg width="100%" viewBox={`0 0 ${w} ${h}`} className="mt-3">
      <rect x={pad} y={pad} width={w - 2 * pad} height={h - 2 * pad} fill="rgba(127,127,127,0.06)" stroke="var(--color-line)" />
      <line x1={halfX} y1={pad} x2={halfX} y2={h - pad} stroke="#888" strokeDasharray="3 3" opacity={0.5} />
      <line x1={cutX} y1={pad} x2={cutX} y2={h - pad} stroke="#e0533d" strokeWidth={2} />
      <polyline points={pts} fill="none" stroke="#6366f1" strokeWidth={1.6} />
      <text x={cutX + 3} y={pad + 12} fontSize={11} fill="#e0533d">截止 {spec.cutoff.toFixed(2)}</text>
      <text x={pad} y={pad - 8} fontSize={11} fill="var(--app-fg)" opacity={0.6}>白化径向频谱(越早跌落=越糊/被放大)</text>
      <text x={pad} y={h - pad + 13} fontSize={10} fill="#888">0</text>
      <text x={w - pad - 16} y={h - pad + 13} fontSize={10} fill="#888">Nyq</text>
    </svg>
  )
}

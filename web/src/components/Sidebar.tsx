import { useEffect, useRef, useState } from 'react'
import { Loader2, Search, Star, Wand2, X, ImagePlus } from 'lucide-react'
import { api, MODEL_PRESETS } from '../api'
import type { Config, Mode } from '../types'
import SizePicker, { sizeOf, type SizeState } from './SizePicker'

interface Props {
  config: Config
  mode: Mode
  setMode: (m: Mode) => void
  prompt: string
  setPrompt: (s: string) => void
  editSources: string[]
  setEditSources: (f: string[]) => void
  onTasksChanged: () => void
  onHistoryChanged: () => void
  onLightbox: (url: string) => void
}

const MODES: { key: Mode; label: string; sub: string }[] = [
  { key: 'images', label: '文生图', sub: 'images' },
  { key: 'chat', label: '对话生图', sub: 'chat' },
  { key: 'edit', label: '图片编辑', sub: 'edits' },
]

export default function Sidebar(p: Props) {
  const { config } = p
  const [model, setModel] = useState(config.model || 'gpt-image-2')
  const [customModel, setCustomModel] = useState('')
  const [size, setSize] = useState<SizeState>({ ratio: '1:1', tier: '1K', custom: '' })
  const [quality, setQuality] = useState(config.default_quality || 'high')
  const [count, setCount] = useState(1)
  const [repeat, setRepeat] = useState(1)
  const [refFiles, setRefFiles] = useState<File[]>([])
  const [busy, setBusy] = useState(false)
  const [status, setStatus] = useState<{ msg: string; kind: 'ok' | 'err' | 'info' } | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)
  const revRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    setModel(config.model || 'gpt-image-2')
    setQuality(config.default_quality || 'high')
  }, [config.model, config.default_quality, config.active_profile])

  const modelList = MODEL_PRESETS.includes(model) || !model ? MODEL_PRESETS : [model, ...MODEL_PRESETS]
  const realModel = model === '__custom' ? customModel.trim() || 'gpt-image-2' : model
  // 整段输入作为一条提示词,不按行拆分(段内换行原样保留)
  const trimmed = p.prompt.trim()
  const prompts = trimmed ? [trimmed] : []

  function say(msg: string, kind: 'ok' | 'err' | 'info') {
    setStatus({ msg, kind })
  }

  async function doReverse(e: React.ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0]
    if (!f) return
    say('正在反推提示词…', 'info')
    const fd = new FormData()
    fd.append('image', f)
    try {
      const { prompt } = await api.reversePrompt(fd)
      p.setPrompt(prompt)
      say('反推完成 ✓ 已填入提示词', 'ok')
    } catch (e) {
      say('反推错误:' + (e as Error).message, 'err')
    }
    if (revRef.current) revRef.current.value = ''
  }

  async function saveFavorite() {
    const first = prompts[0]
    if (!first) return say('提示词为空,无法收藏', 'err')
    const name = window.prompt('给这个模板起个名字(可留空):', '') ?? ''
    await api.addFavorite(first, name)
    say('已收藏 ★', 'ok')
  }

  async function generate() {
    if (!prompts.length) return say('请输入提示词', 'err')
    setBusy(true)
    try {
      if (p.mode === 'edit') {
        if (!refFiles.length && !p.editSources.length) {
          setBusy(false)
          return say('请上传参考图,或从历史里「继续创作」', 'err')
        }
        say('编辑生成中…', 'info')
        const fd = new FormData()
        fd.append('prompt', prompts[0])
        fd.append('size', sizeOf(size))
        fd.append('quality', quality)
        fd.append('n', String(count))
        fd.append('model', realModel)
        if (p.editSources.length) fd.append('source', p.editSources.join(','))
        refFiles.forEach((f) => fd.append('images', f))
        const data = await api.edit(fd)
        say(`完成 ✓ 共 ${data.images.length} 张`, 'ok')
        p.setEditSources([])
        setRefFiles([])
        p.onHistoryChanged()
      } else {
        const data = await api.generate({
          prompts,
          size: sizeOf(size),
          quality,
          n: count,
          repeat,
          request_format: p.mode,
          model: realModel,
        })
        say(`已加入队列:${prompts.length} 条 × ${repeat} = ${data.tasks.length} 个任务`, 'ok')
        p.onTasksChanged()
      }
    } catch (e) {
      say('错误:' + (e as Error).message, 'err')
    } finally {
      setBusy(false)
    }
  }

  return (
    <aside className="glass flex w-[400px] shrink-0 flex-col overflow-y-auto border-r border-[var(--color-line)] p-5">
      {/* 模式 */}
      <div className="grid grid-cols-3 gap-1.5 rounded-2xl bg-[var(--color-ink-800)] p-1.5">
        {MODES.map((m) => (
          <button
            key={m.key}
            onClick={() => p.setMode(m.key)}
            className={`flex flex-col items-center rounded-xl py-2 text-[13px] font-medium transition-all ${
              p.mode === m.key
                ? 'bg-gradient-to-br from-brand-500 to-violet-500 text-white shadow-lg'
                : 'text-slate-400 hover:text-slate-200'
            }`}
          >
            {m.label}
            <span className="text-[10px] opacity-70">{m.sub}</span>
          </button>
        ))}
      </div>

      {/* 模型 */}
      <label className="lbl">模型</label>
      <select className="field" value={model} onChange={(e) => setModel(e.target.value)}>
        {modelList.map((m) => (
          <option key={m} value={m}>
            {m}
            {m.includes('nano') || m.includes('gemini') ? ' · Nano Banana/Gemini' : ''}
          </option>
        ))}
        <option value="__custom">自定义…</option>
      </select>
      {model === '__custom' && (
        <input
          className="field mt-2"
          placeholder="自定义模型名,如 gemini-2.5-flash-image"
          value={customModel}
          onChange={(e) => setCustomModel(e.target.value)}
        />
      )}

      {/* 编辑模式上传 */}
      {p.mode === 'edit' && (
        <>
          <label className="lbl">上传参考图(可多张)</label>
          <input
            ref={fileRef}
            type="file"
            accept="image/*"
            multiple
            className="hidden"
            onChange={(e) => setRefFiles(Array.from(e.target.files || []))}
          />
          <button className="btn btn-ghost w-full" onClick={() => fileRef.current?.click()}>
            <ImagePlus size={16} /> 选择图片{refFiles.length ? ` · 已选 ${refFiles.length}` : ''}
          </button>
          {!!refFiles.length && (
            <div className="mt-2 flex flex-wrap gap-2">
              {refFiles.map((f, i) => (
                <img
                  key={i}
                  src={URL.createObjectURL(f)}
                  className="h-14 w-14 rounded-lg border border-[var(--color-line)] object-cover"
                />
              ))}
            </div>
          )}
        </>
      )}

      {/* 继续创作底图 */}
      {p.mode === 'edit' && !!p.editSources.length && (
        <>
          <label className="lbl">继续创作的底图(来自历史结果)</label>
          <div className="flex flex-wrap gap-2">
            {p.editSources.map((f, i) => (
              <div key={i} className="relative">
                <img
                  src={`/outputs/${f}`}
                  className="h-14 w-14 rounded-lg border border-[var(--color-line)] object-cover"
                />
                <button
                  onClick={() => p.setEditSources(p.editSources.filter((_, j) => j !== i))}
                  className="absolute -right-1.5 -top-1.5 grid h-5 w-5 place-items-center rounded-full bg-rose-500 text-white"
                >
                  <X size={12} />
                </button>
              </div>
            ))}
          </div>
        </>
      )}

      {/* 提示词 */}
      <div className="mt-4 flex items-center justify-between">
        <label className="text-xs font-medium text-slate-400">
          提示词 Prompt<span className="ml-1 text-slate-600">整段作为一条</span>
        </label>
        <div className="flex gap-1.5">
          <input ref={revRef} type="file" accept="image/*" className="hidden" onChange={doReverse} />
          <button className="btn btn-ghost !px-2 !py-1 text-xs" onClick={() => revRef.current?.click()}>
            <Search size={13} /> 反推
          </button>
          <button className="btn btn-ghost !px-2 !py-1 text-xs" onClick={saveFavorite}>
            <Star size={13} /> 收藏
          </button>
        </div>
      </div>
      <textarea
        value={p.prompt}
        onChange={(e) => p.setPrompt(e.target.value)}
        placeholder={'输入提示词,整段作为一条任务(换行不拆分)。\n点「反推」可上传一张图自动生成提示词。'}
        className="field mt-1.5 min-h-[108px] resize-y leading-relaxed"
      />
      <div className="mt-1.5 text-xs text-slate-500">{repeat > 1 ? `将生成 ${repeat} 次` : ' '}</div>

      {/* 尺寸 */}
      <div className="mt-1">
        <SizePicker value={size} onChange={setSize} />
      </div>

      {/* 质量 / 张数 / 重复 */}
      <div className="mt-1 grid grid-cols-3 gap-2.5">
        <div>
          <label className="lbl">质量</label>
          <select className="field" value={quality} onChange={(e) => setQuality(e.target.value)}>
            {['high', 'medium', 'low', 'auto'].map((q) => (
              <option key={q}>{q}</option>
            ))}
          </select>
        </div>
        <div>
          <label className="lbl">每任务张数</label>
          <select className="field" value={count} onChange={(e) => setCount(+e.target.value)}>
            {[1, 2, 3, 4].map((n) => (
              <option key={n}>{n}</option>
            ))}
          </select>
        </div>
        <div>
          <label className="lbl">每条重复</label>
          <select className="field" value={repeat} onChange={(e) => setRepeat(+e.target.value)}>
            {[1, 2, 3, 5, 10].map((n) => (
              <option key={n}>{n}</option>
            ))}
          </select>
        </div>
      </div>

      <button onClick={generate} disabled={busy} className="btn btn-primary mt-5 w-full py-3 text-[15px]">
        {busy ? <Loader2 size={17} className="spin" /> : <Wand2 size={17} />}
        {p.mode === 'edit' ? '生成(编辑)' : '加入队列生成'}
      </button>

      {status && (
        <div
          className={`mt-3 whitespace-pre-wrap text-[13px] ${
            status.kind === 'err' ? 'text-rose-400' : status.kind === 'ok' ? 'text-emerald-400' : 'text-slate-400'
          }`}
        >
          {status.msg}
        </div>
      )}
    </aside>
  )
}

import { SIZE_MATRIX } from '../api'

export interface SizeState {
  ratio: string
  tier: string
  custom: string
}

export function sizeOf(s: SizeState): string {
  if (s.custom.trim()) return s.custom.trim()
  if (s.ratio === 'Auto') return 'auto'
  return SIZE_MATRIX[s.ratio][s.tier]
}

export function readout(s: SizeState): string {
  if (s.ratio === 'Auto' && !s.custom.trim()) return 'auto(模型决定)'
  return sizeOf(s).replace('x', '×')
}

interface Props {
  value: SizeState
  onChange: (s: SizeState) => void
}

export default function SizePicker({ value, onChange }: Props) {
  const ratios = ['Auto', ...Object.keys(SIZE_MATRIX)]
  const auto = value.ratio === 'Auto'
  return (
    <div>
      <label className="lbl">画面比例</label>
      <div className="flex flex-wrap gap-2">
        {ratios.map((r) => (
          <div
            key={r}
            className={`chip ${r === value.ratio ? 'chip-on' : ''}`}
            onClick={() => onChange({ ...value, ratio: r, custom: '' })}
          >
            {r}
          </div>
        ))}
      </div>
      <label className="lbl">分辨率档位</label>
      <div className="flex flex-wrap gap-2">
        {['1K', '2K', '4K'].map((t) => (
          <div
            key={t}
            className={`chip ${t === value.tier ? 'chip-on' : ''} ${auto ? 'chip-dim' : ''}`}
            onClick={() => onChange({ ...value, tier: t, custom: '' })}
          >
            {t}
          </div>
        ))}
      </div>
      <div className="mt-3 flex items-center gap-2 text-xs text-slate-400">
        <span>
          实际尺寸:<b className="text-brand-400">{readout(value)}</b>
        </span>
        <span className="ml-auto">自定义</span>
        <input
          type="text"
          value={value.custom}
          onChange={(e) => onChange({ ...value, custom: e.target.value })}
          placeholder="2304x1792"
          className="field !w-32 !py-1.5 !text-xs"
        />
      </div>
    </div>
  )
}

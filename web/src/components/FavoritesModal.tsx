import { useEffect, useState } from 'react'
import { Trash2 } from 'lucide-react'
import { api } from '../api'
import type { Favorite } from '../types'
import Modal from './Modal'

interface Props {
  open: boolean
  onClose: () => void
  onUse: (prompt: string) => void
}

export default function FavoritesModal({ open, onClose, onUse }: Props) {
  const [list, setList] = useState<Favorite[]>([])

  async function load() {
    setList((await api.listFavorites()).favorites)
  }
  useEffect(() => {
    if (open) load()
  }, [open])

  async function del(id: number) {
    await api.deleteFavorite(id)
    load()
  }

  return (
    <Modal open={open} onClose={onClose} title="提示词模板 / 收藏" subtitle="点「使用」把提示词填入输入框。" width={560}>
      {list.length === 0 ? (
        <div className="card text-sm text-slate-500">还没有收藏的提示词。点输入框旁「收藏」添加。</div>
      ) : (
        <div className="grid gap-2">
          {list.map((f) => (
            <div key={f.id} className="flex items-center gap-3 rounded-xl border border-[var(--color-line)] bg-[var(--color-ink-800)] p-3">
              <div className="min-w-0 flex-1">
                <div className="text-sm font-semibold text-slate-200">{f.name || '(未命名)'}</div>
                <div className="truncate text-xs text-slate-500" title={f.prompt}>{f.prompt}</div>
              </div>
              <button className="btn btn-primary !px-3 !py-1.5 text-xs" onClick={() => { onUse(f.prompt); onClose() }}>
                使用
              </button>
              <button className="btn btn-ghost !px-2 !py-1.5 text-xs hover:!border-rose-500 hover:!text-rose-400" onClick={() => del(f.id)}>
                <Trash2 size={13} />
              </button>
            </div>
          ))}
        </div>
      )}
    </Modal>
  )
}

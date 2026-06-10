import { X } from 'lucide-react'
import type { ReactNode } from 'react'

interface Props {
  open: boolean
  onClose: () => void
  title: ReactNode
  subtitle?: ReactNode
  children: ReactNode
  width?: number
  footer?: ReactNode
}

export default function Modal({ open, onClose, title, subtitle, children, width = 560, footer }: Props) {
  if (!open) return null
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
      onClick={(e) => e.target === e.currentTarget && onClose()}
    >
      <div
        className="glass fadeup relative flex max-h-[88vh] w-full flex-col overflow-hidden rounded-2xl shadow-2xl"
        style={{ maxWidth: width }}
      >
        <button
          onClick={onClose}
          className="absolute right-4 top-4 z-10 grid h-9 w-9 place-items-center rounded-lg text-slate-400 hover:bg-white/5 hover:text-white"
        >
          <X size={18} />
        </button>
        <div className="px-6 pt-6 pb-3">
          <h2 className="text-lg font-semibold text-white">{title}</h2>
          {subtitle && <p className="mt-1 text-xs text-slate-400">{subtitle}</p>}
        </div>
        <div className="flex-1 overflow-y-auto px-6 pb-4">{children}</div>
        {footer && <div className="border-t border-[var(--color-line)] px-6 py-4">{footer}</div>}
      </div>
    </div>
  )
}

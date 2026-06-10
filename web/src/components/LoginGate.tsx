import { useState } from 'react'
import { KeyRound, Loader2, Sparkles } from 'lucide-react'
import { api } from '../api'

export default function LoginGate({ onSuccess }: { onSuccess: () => void }) {
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!password) return
    setLoading(true)
    setErr('')
    try {
      await api.login(password)
      onSuccess()
    } catch (e) {
      setErr((e as Error).message)
      setLoading(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <div className="fadeup w-full max-w-sm">
        <div className="mb-7 text-center">
          <div className="mx-auto mb-4 grid h-16 w-16 place-items-center rounded-2xl bg-gradient-to-br from-brand-500 to-fuchsia-500 shadow-xl shadow-indigo-500/30">
            <Sparkles className="text-white" size={30} />
          </div>
          <h1 className="text-2xl font-bold tracking-tight text-white">STCS 生图压测台</h1>
          <p className="mt-1.5 text-sm text-slate-400">GPT / Gemini 生图 · 中转站压测</p>
        </div>
        <form onSubmit={submit} className="glass rounded-2xl p-6 shadow-2xl">
          <label className="mb-2 block text-sm font-medium text-slate-300">访问密码</label>
          <div className="relative">
            <KeyRound
              size={17}
              className="pointer-events-none absolute left-3.5 top-1/2 -translate-y-1/2 text-slate-500"
            />
            <input
              autoFocus
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="请输入密码进入测试页"
              className="field !pl-10"
            />
          </div>
          {err && <p className="mt-3 text-sm text-rose-400">{err}</p>}
          <button type="submit" disabled={loading} className="btn btn-primary mt-5 w-full py-2.5">
            {loading ? <Loader2 size={17} className="spin" /> : <KeyRound size={16} />}
            {loading ? '验证中…' : '进入'}
          </button>
        </form>
        <p className="mt-5 text-center text-xs text-slate-600">
          密码由部署方通过 STCS_PASSWORD 环境变量配置
        </p>
      </div>
    </div>
  )
}

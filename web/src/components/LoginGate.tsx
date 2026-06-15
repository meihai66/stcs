import { useCallback, useEffect, useState } from 'react'
import { KeyRound, Loader2, RefreshCw, Sparkles, User } from 'lucide-react'
import { api } from '../api'

export default function LoginGate({ onSuccess }: { onSuccess: () => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [captcha, setCaptcha] = useState('')
  const [captchaId, setCaptchaId] = useState('')
  const [captchaImg, setCaptchaImg] = useState('')
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)

  const loadCaptcha = useCallback(async () => {
    try {
      const c = await api.captcha()
      setCaptchaId(c.id)
      setCaptchaImg(c.image)
      setCaptcha('')
    } catch {
      /* ignore */
    }
  }, [])

  useEffect(() => {
    loadCaptcha()
  }, [loadCaptcha])

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!username || !password || !captcha) return
    setLoading(true)
    setErr('')
    try {
      await api.login(username, password, captchaId, captcha)
      onSuccess()
    } catch (e) {
      setErr((e as Error).message)
      setLoading(false)
      loadCaptcha()
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <div className="fadeup w-full max-w-sm">
        <div className="mb-7 text-center">
          <div className="mx-auto mb-4 grid h-16 w-16 place-items-center rounded-2xl bg-gradient-to-br from-brand-500 to-fuchsia-500 shadow-xl shadow-indigo-500/30">
            <Sparkles className="text-white" size={30} />
          </div>
          <h1 className="text-2xl font-bold tracking-tight text-slate-100">STCS 生图压测台</h1>
          <p className="mt-1.5 text-sm text-slate-400">GPT / Gemini 生图 · 中转站压测</p>
        </div>
        <form onSubmit={submit} className="glass rounded-2xl p-6 shadow-2xl">
          <label className="mb-2 block text-sm font-medium text-slate-300">用户名</label>
          <div className="relative">
            <User size={17} className="pointer-events-none absolute left-3.5 top-1/2 -translate-y-1/2 text-slate-500" />
            <input
              autoFocus
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="请输入用户名"
              className="field !pl-10"
            />
          </div>

          <label className="mb-2 mt-4 block text-sm font-medium text-slate-300">密码</label>
          <div className="relative">
            <KeyRound size={17} className="pointer-events-none absolute left-3.5 top-1/2 -translate-y-1/2 text-slate-500" />
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="请输入密码"
              className="field !pl-10"
            />
          </div>

          <label className="mb-2 mt-4 block text-sm font-medium text-slate-300">验证码</label>
          <div className="flex items-center gap-2">
            <input
              value={captcha}
              onChange={(e) => setCaptcha(e.target.value)}
              placeholder="输入图中数字"
              inputMode="numeric"
              className="field flex-1"
            />
            <button
              type="button"
              onClick={loadCaptcha}
              title="看不清?点击换一张"
              className="flex h-[42px] shrink-0 items-center overflow-hidden rounded-xl border border-[var(--color-line)] bg-white"
            >
              {captchaImg ? (
                <img src={captchaImg} alt="验证码" className="h-full" />
              ) : (
                <RefreshCw size={16} className="mx-6 text-slate-400" />
              )}
            </button>
          </div>

          {err && <p className="mt-3 text-sm text-rose-400">{err}</p>}
          <button type="submit" disabled={loading} className="btn btn-primary mt-5 w-full py-2.5">
            {loading ? <Loader2 size={17} className="spin" /> : <KeyRound size={16} />}
            {loading ? '登录中…' : '登录'}
          </button>
        </form>
        <p className="mt-5 text-center text-xs text-slate-600">账户由管理员创建 · 登录有效期 30 天</p>
      </div>
    </div>
  )
}

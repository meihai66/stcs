import { useCallback, useEffect, useRef, useState } from 'react'
import { LogOut, Settings, ShoppingBag, Sparkles, Star, Zap } from 'lucide-react'
import { api } from './api'
import type { Config, HistoryItem, Mode, Profile, Task } from './types'
import LoginGate from './components/LoginGate'
import Sidebar from './components/Sidebar'
import Workspace from './components/Workspace'
import SettingsModal from './components/SettingsModal'
import FavoritesModal from './components/FavoritesModal'
import MarketModal from './components/MarketModal'
import StressModal from './components/StressModal'

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null)
  const [config, setConfig] = useState<Config | null>(null)
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [active, setActive] = useState('')
  const [tasks, setTasks] = useState<Task[]>([])
  const [history, setHistory] = useState<HistoryItem[]>([])

  const [mode, setMode] = useState<Mode>('images')
  const [prompt, setPrompt] = useState('')
  const [editSources, setEditSources] = useState<string[]>([])

  const [lightbox, setLightbox] = useState<string | null>(null)
  const [modal, setModal] = useState<'' | 'settings' | 'favorites' | 'market' | 'stress'>('')
  const lastActive = useRef(0)

  const loadConfig = useCallback(async () => {
    const cfg = await api.getConfig()
    setConfig(cfg)
    const pr = await api.listProfiles()
    setProfiles(pr.profiles)
    setActive(pr.active)
    return cfg
  }, [])

  const loadTasks = useCallback(async () => setTasks((await api.listTasks()).tasks), [])
  const loadHistory = useCallback(async () => setHistory((await api.listHistory()).history), [])

  // 初始:检查登录态
  useEffect(() => {
    api
      .authStatus()
      .then((s) => setAuthed(s.authed))
      .catch(() => setAuthed(false))
  }, [])

  // 登录后加载数据
  useEffect(() => {
    if (!authed) return
    ;(async () => {
      const cfg = await loadConfig()
      if (cfg.request_format) setMode(cfg.request_format === 'chat' ? 'chat' : 'images')
      await loadTasks()
      await loadHistory()
      if (!cfg.has_api_key || !cfg.base_url) setModal('settings')
    })()
  }, [authed, loadConfig, loadTasks, loadHistory])

  // 轮询任务
  useEffect(() => {
    if (!authed) return
    const id = setInterval(async () => {
      try {
        const { tasks } = await api.listTasks()
        setTasks(tasks)
        const act = tasks.filter((t) => t.status === 'queued' || t.status === 'running').length
        if (lastActive.current > 0 && act < lastActive.current) loadHistory()
        lastActive.current = act
      } catch {
        /* ignore */
      }
    }, 1500)
    return () => clearInterval(id)
  }, [authed, loadHistory])

  async function switchProfile(name: string) {
    await api.activateProfile(name)
    setActive(name)
    await loadConfig()
  }

  async function logout() {
    await api.logout()
    setAuthed(false)
    setConfig(null)
  }

  function continueFrom(filename: string) {
    setEditSources((s) => (s.includes(filename) ? s : [...s, filename]))
    setMode('edit')
    window.scrollTo(0, 0)
  }

  if (authed === null) {
    return (
      <div className="grid min-h-screen place-items-center text-slate-500">
        <Sparkles className="spin" />
      </div>
    )
  }
  if (!authed) return <LoginGate onSuccess={() => setAuthed(true)} />
  if (!config) {
    return (
      <div className="grid min-h-screen place-items-center text-slate-500">
        <Sparkles className="spin" />
      </div>
    )
  }

  return (
    <div className="flex h-screen flex-col">
      {/* 顶栏 */}
      <header className="glass z-20 flex items-center justify-between gap-3 border-b border-[var(--color-line)] px-5 py-3">
        <div className="flex items-center gap-2.5">
          <div className="grid h-9 w-9 place-items-center rounded-xl bg-gradient-to-br from-brand-500 to-fuchsia-500 shadow-lg shadow-indigo-500/30">
            <Sparkles size={18} className="text-white" />
          </div>
          <div>
            <h1 className="text-[15px] font-bold leading-none text-white">STCS 生图压测台</h1>
            <span className="text-[11px] text-slate-500">{config.model || '未配置模型'}</span>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <select
            className="field !w-auto !py-2 max-w-[180px] text-[13px] font-medium"
            value={active}
            onChange={(e) => switchProfile(e.target.value)}
            title="切换中转站配置"
          >
            {profiles.length === 0 && <option value="">未配置</option>}
            {profiles.map((p) => (
              <option key={p.name} value={p.name}>{p.name}</option>
            ))}
          </select>
          <button className="btn btn-ghost" onClick={() => setModal('market')}>
            <ShoppingBag size={15} /> 市场
          </button>
          <button className="btn btn-ghost" onClick={() => setModal('stress')}>
            <Zap size={15} /> 压测
          </button>
          <button className="btn btn-ghost" onClick={() => setModal('favorites')}>
            <Star size={15} /> 模板
          </button>
          <button className="btn btn-ghost" onClick={() => setModal('settings')}>
            <Settings size={15} /> 设置
          </button>
          <button className="btn btn-ghost !px-2.5" onClick={logout} title="退出登录">
            <LogOut size={15} />
          </button>
        </div>
      </header>

      {/* 主体 */}
      <div className="flex min-h-0 flex-1">
        <Sidebar
          config={config}
          mode={mode}
          setMode={setMode}
          prompt={prompt}
          setPrompt={setPrompt}
          editSources={editSources}
          setEditSources={setEditSources}
          onTasksChanged={loadTasks}
          onHistoryChanged={loadHistory}
          onLightbox={setLightbox}
        />
        <Workspace
          tasks={tasks}
          history={history}
          onLightbox={setLightbox}
          onContinue={continueFrom}
          onReuse={(p) => { setPrompt(p); window.scrollTo(0, 0) }}
          onDeleteHistory={async (id) => { await api.deleteHistory(id); loadHistory() }}
          onRefreshTasks={loadTasks}
        />
      </div>

      {/* 弹窗 */}
      <SettingsModal
        open={modal === 'settings'}
        onClose={() => setModal('')}
        config={config}
        profiles={profiles}
        active={active}
        onSaved={loadConfig}
      />
      <FavoritesModal open={modal === 'favorites'} onClose={() => setModal('')} onUse={setPrompt} />
      <MarketModal
        open={modal === 'market'}
        onClose={() => setModal('')}
        onApply={(p, m) => { setPrompt(p); setMode(m) }}
        onLightbox={setLightbox}
      />
      <StressModal open={modal === 'stress'} onClose={() => setModal('')} config={config} />

      {/* 灯箱 */}
      {lightbox && (
        <div
          className="fixed inset-0 z-[60] flex items-center justify-center bg-black/90 p-6"
          onClick={() => setLightbox(null)}
        >
          <img src={lightbox} className="max-h-[92vh] max-w-[92vw] rounded-lg" />
        </div>
      )}
    </div>
  )
}

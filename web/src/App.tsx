import { useCallback, useEffect, useRef, useState } from 'react'
import { Download, LogOut, Moon, ScanSearch, ScrollText, Settings, ShoppingBag, Sparkles, Star, Sun, Users, Zap } from 'lucide-react'
import { api } from './api'
import type { Config, HistoryItem, Mode, Profile, Task } from './types'
import LoginGate from './components/LoginGate'
import Sidebar from './components/Sidebar'
import Workspace from './components/Workspace'
import SettingsModal from './components/SettingsModal'
import FavoritesModal from './components/FavoritesModal'
import MarketModal from './components/MarketModal'
import StressModal from './components/StressModal'
import RequestLogsModal from './components/RequestLogsModal'
import UsersModal from './components/UsersModal'
import InspectModal from './components/InspectModal'

const HISTORY_PAGE_SIZE = 12

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null)
  const [config, setConfig] = useState<Config | null>(null)
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [active, setActive] = useState('')
  const [tasks, setTasks] = useState<Task[]>([])
  const [history, setHistory] = useState<HistoryItem[]>([])
  const [historyTotal, setHistoryTotal] = useState(0)
  const [historyPage, setHistoryPage] = useState(1)
  const historyPageRef = useRef(1)

  const [mode, setMode] = useState<Mode>('images')
  const [prompt, setPrompt] = useState('')
  const [editSources, setEditSources] = useState<string[]>([])

  const [lightbox, setLightbox] = useState<string | null>(null)
  const [modal, setModal] = useState<'' | 'settings' | 'favorites' | 'market' | 'stress' | 'logs' | 'users' | 'inspect'>('')
  const [theme, setTheme] = useState<'dark' | 'light'>(
    () => (localStorage.getItem('stcs-theme') === 'light' ? 'light' : 'dark'),
  )
  const [viewAll, setViewAll] = useState(false)
  const viewAllRef = useRef(false)
  const lastActive = useRef(0)
  const isAdmin = config?.role === 'admin'

  useEffect(() => {
    document.documentElement.classList.toggle('light', theme === 'light')
    localStorage.setItem('stcs-theme', theme)
  }, [theme])

  const loadConfig = useCallback(async () => {
    const cfg = await api.getConfig()
    setConfig(cfg)
    const pr = await api.listProfiles()
    setProfiles(pr.profiles)
    setActive(pr.active)
    return cfg
  }, [])

  const loadTasks = useCallback(async () => setTasks((await api.listTasks(viewAllRef.current)).tasks), [])
  const loadHistory = useCallback(async () => {
    const res = await api.listHistory(viewAllRef.current, historyPageRef.current, HISTORY_PAGE_SIZE)
    // 当前页被删空且非第一页(如删除/清理后)→ 回退到最后一页
    if (res.history.length === 0 && res.total > 0 && historyPageRef.current > 1) {
      const last = Math.max(1, Math.ceil(res.total / HISTORY_PAGE_SIZE))
      historyPageRef.current = last
      setHistoryPage(last)
      const res2 = await api.listHistory(viewAllRef.current, last, HISTORY_PAGE_SIZE)
      setHistory(res2.history)
      setHistoryTotal(res2.total)
      return
    }
    setHistory(res.history)
    setHistoryTotal(res.total)
  }, [])

  function goHistoryPage(page: number) {
    const total = Math.max(1, Math.ceil(historyTotal / HISTORY_PAGE_SIZE))
    const p = Math.min(Math.max(1, page), total)
    historyPageRef.current = p
    setHistoryPage(p)
    loadHistory()
  }

  function toggleViewAll() {
    const v = !viewAll
    setViewAll(v)
    viewAllRef.current = v
    historyPageRef.current = 1
    setHistoryPage(1)
    loadTasks()
    loadHistory()
  }

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
        const { tasks } = await api.listTasks(viewAllRef.current)
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
            <h1 className="text-[15px] font-bold leading-none text-slate-100">STCS 生图压测台</h1>
            <span className="text-[11px] text-slate-500">
              {config.username}
              {isAdmin && <span className="ml-1 rounded bg-brand-500/20 px-1 text-brand-400">管理员</span>}
              {' · '}
              {config.model || '未配置模型'}
            </span>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {isAdmin && (
            <button
              className={`btn btn-ghost !px-2.5 ${viewAll ? '!border-brand-500 !text-brand-400' : ''}`}
              onClick={toggleViewAll}
              title="切换:仅看自己 / 看全部用户的数据"
            >
              {viewAll ? '全部用户' : '仅我'}
            </button>
          )}
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
          <button className="btn btn-ghost" onClick={() => setModal('inspect')} title="检测图片是否真高清/被放大/超分">
            <ScanSearch size={15} /> 检测
          </button>
          <button className="btn btn-ghost" onClick={() => setModal('logs')} title="查看 200 无图的请求日志">
            <ScrollText size={15} /> 日志
          </button>
          <button className="btn btn-ghost" onClick={() => setModal('favorites')}>
            <Star size={15} /> 模板
          </button>
          {isAdmin && (
            <button className="btn btn-ghost" onClick={() => setModal('users')}>
              <Users size={15} /> 用户
            </button>
          )}
          <button className="btn btn-ghost" onClick={() => setModal('settings')}>
            <Settings size={15} /> 设置
          </button>
          <button
            className="btn btn-ghost !px-2.5"
            onClick={() => setTheme(theme === 'light' ? 'dark' : 'light')}
            title={theme === 'light' ? '切换到深色主题' : '切换到浅色主题'}
          >
            {theme === 'light' ? <Moon size={15} /> : <Sun size={15} />}
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
          historyTotal={historyTotal}
          historyPage={historyPage}
          historyPageSize={HISTORY_PAGE_SIZE}
          onHistoryPage={goHistoryPage}
          showOwner={isAdmin && viewAll}
          onLightbox={setLightbox}
          onContinue={continueFrom}
          onReuse={(p) => { setPrompt(p); window.scrollTo(0, 0) }}
          onDeleteHistory={async (id) => { await api.deleteHistory(id); loadHistory() }}
          onRefreshTasks={loadTasks}
          onClearTasks={async () => { await api.clearTasks(); loadTasks() }}
          onClearHistory={async () => { historyPageRef.current = 1; setHistoryPage(1); await api.clearHistory(); loadHistory() }}
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
      <StressModal
        open={modal === 'stress'}
        onClose={() => setModal('')}
        config={config}
        onLightbox={setLightbox}
      />
      <RequestLogsModal open={modal === 'logs'} onClose={() => setModal('')} isAdmin={!!isAdmin} />
      <UsersModal open={modal === 'users'} onClose={() => setModal('')} />
      <InspectModal open={modal === 'inspect'} onClose={() => setModal('')} />

      {/* 灯箱 */}
      {lightbox && (
        <div
          className="fixed inset-0 z-[60] flex items-center justify-center bg-black/90 p-6"
          onClick={() => setLightbox(null)}
        >
          <img src={lightbox} className="max-h-[88vh] max-w-[92vw] rounded-lg" />
          <a
            href={lightbox}
            download={lightbox.split('/').pop() || 'image.png'}
            className="btn btn-primary absolute bottom-6 left-1/2 -translate-x-1/2"
            onClick={(e) => e.stopPropagation()}
          >
            <Download size={15} /> 保存图片
          </a>
        </div>
      )}
    </div>
  )
}

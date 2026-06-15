import { useEffect, useState } from 'react'
import { KeyRound, Trash2, UserPlus } from 'lucide-react'
import { api } from '../api'
import type { GlobalSettings, User } from '../types'
import Modal from './Modal'

interface Props {
  open: boolean
  onClose: () => void
}

export default function UsersModal({ open, onClose }: Props) {
  const [users, setUsers] = useState<User[]>([])
  const [settings, setSettings] = useState<GlobalSettings | null>(null)
  const [msg, setMsg] = useState<{ text: string; ok: boolean } | null>(null)
  // 新建用户表单
  const [nu, setNu] = useState({ username: '', password: '', role: 'user', image_limit: 200 })
  // 全局设置可编辑值
  const [logLimit, setLogLimit] = useState(1000)
  const [defImg, setDefImg] = useState(200)
  const [conc, setConc] = useState(3)
  const [serverKey, setServerKey] = useState('')

  async function load() {
    try {
      const [u, s] = await Promise.all([api.listUsers(), api.getSettings()])
      setUsers(u.users || [])
      setSettings(s)
      setLogLimit(s.log_limit)
      setDefImg(s.default_image_limit)
      setConc(s.concurrency)
    } catch (e) {
      flash((e as Error).message, false)
    }
  }

  useEffect(() => {
    if (open) {
      setMsg(null)
      setServerKey('')
      load()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  function flash(text: string, ok: boolean) {
    setMsg({ text, ok })
    window.setTimeout(() => setMsg(null), 3000)
  }

  async function createUser() {
    if (!nu.username.trim() || !nu.password.trim()) return flash('用户名和密码不能为空', false)
    try {
      await api.createUser({ ...nu, username: nu.username.trim(), password: nu.password })
      setNu({ username: '', password: '', role: 'user', image_limit: defImg })
      flash('已创建用户 ✓', true)
      load()
    } catch (e) {
      flash((e as Error).message, false)
    }
  }

  async function saveLimit(u: User, limit: number) {
    try {
      await api.updateUser(u.id, { image_limit: limit })
      flash(`已更新 ${u.username} 的图片上限`, true)
      load()
    } catch (e) {
      flash((e as Error).message, false)
    }
  }

  async function resetPassword(u: User) {
    const p = window.prompt(`为「${u.username}」设置新密码:`, '')
    if (!p) return
    try {
      await api.updateUser(u.id, { password: p })
      flash(`已重置 ${u.username} 的密码`, true)
    } catch (e) {
      flash((e as Error).message, false)
    }
  }

  async function toggleRole(u: User) {
    const role = u.role === 'admin' ? 'user' : 'admin'
    try {
      await api.updateUser(u.id, { role })
      flash(`${u.username} 已设为${role === 'admin' ? '管理员' : '普通用户'}`, true)
      load()
    } catch (e) {
      flash((e as Error).message, false)
    }
  }

  async function removeUser(u: User) {
    if (!window.confirm(`删除用户「${u.username}」?其历史/图片/收藏将一并删除,不可恢复。`)) return
    try {
      await api.deleteUser(u.id)
      flash(`已删除 ${u.username}`, true)
      load()
    } catch (e) {
      flash((e as Error).message, false)
    }
  }

  async function saveSettings() {
    try {
      const body: Record<string, unknown> = { log_limit: logLimit, default_image_limit: defImg, concurrency: conc }
      if (serverKey.trim()) body.server_api_key = serverKey.trim()
      await api.setSettings(body)
      setServerKey('')
      flash('全局设置已保存 ✓', true)
      load()
    } catch (e) {
      flash((e as Error).message, false)
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="👥 用户管理" subtitle="新建/管理用户与全局配额(仅管理员)。" width={920}>
      {msg && (
        <div className={`mb-3 rounded-lg px-3 py-2 text-sm ${msg.ok ? 'bg-emerald-500/15 text-emerald-400' : 'bg-rose-500/15 text-rose-400'}`}>
          {msg.text}
        </div>
      )}

      {/* 新建用户 */}
      <div className="card mb-5">
        <div className="mb-2.5 text-xs font-semibold uppercase tracking-wider text-slate-500">新建用户</div>
        <div className="flex flex-wrap items-end gap-2">
          <div className="flex-1 min-w-[140px]">
            <label className="lbl !mt-0">用户名</label>
            <input className="field" value={nu.username} onChange={(e) => setNu({ ...nu, username: e.target.value })} />
          </div>
          <div className="flex-1 min-w-[140px]">
            <label className="lbl !mt-0">密码</label>
            <input className="field" type="text" value={nu.password} onChange={(e) => setNu({ ...nu, password: e.target.value })} />
          </div>
          <div className="w-24">
            <label className="lbl !mt-0">角色</label>
            <select className="field" value={nu.role} onChange={(e) => setNu({ ...nu, role: e.target.value })}>
              <option value="user">普通</option>
              <option value="admin">管理员</option>
            </select>
          </div>
          <div className="w-28">
            <label className="lbl !mt-0">图片上限</label>
            <input
              className="field"
              type="number"
              value={nu.image_limit}
              onChange={(e) => setNu({ ...nu, image_limit: Number(e.target.value) })}
            />
          </div>
          <button className="btn btn-primary !py-2.5" onClick={createUser}>
            <UserPlus size={15} /> 创建
          </button>
        </div>
      </div>

      {/* 用户列表 */}
      <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-slate-500">用户列表({users.length})</div>
      <div className="mb-5 flex flex-col gap-2">
        {users.map((u) => (
          <div key={u.id} className="card flex flex-wrap items-center gap-2.5">
            <span className="font-medium text-slate-200">{u.username}</span>
            <button
              onClick={() => toggleRole(u)}
              title="点击切换角色"
              className={`rounded-full px-1.5 py-0.5 text-[10px] ${u.role === 'admin' ? 'bg-brand-500 text-white' : 'bg-slate-500/30 text-slate-300'}`}
            >
              {u.role === 'admin' ? '管理员' : '普通'}
            </button>
            <div className="ml-auto flex items-center gap-2">
              <span className="text-[11px] text-slate-500">图片上限</span>
              <input
                className="field !w-20 !py-1 text-xs"
                type="number"
                defaultValue={u.image_limit}
                onBlur={(e) => {
                  const v = Number(e.target.value)
                  if (v !== u.image_limit) saveLimit(u, v)
                }}
              />
              <button className="btn btn-ghost !px-2 !py-1 text-xs" onClick={() => resetPassword(u)}>
                <KeyRound size={12} /> 改密
              </button>
              <button
                className="btn btn-ghost !px-2 !py-1 text-xs hover:!border-rose-500 hover:!text-rose-400"
                onClick={() => removeUser(u)}
              >
                <Trash2 size={12} /> 删除
              </button>
            </div>
          </div>
        ))}
      </div>

      {/* 全局设置 */}
      {settings && (
        <>
          <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-slate-500">全局设置</div>
          <div className="card flex flex-wrap items-end gap-3">
            <div className="w-32">
              <label className="lbl !mt-0">日志保存条数</label>
              <input className="field" type="number" value={logLimit} onChange={(e) => setLogLimit(Number(e.target.value))} />
            </div>
            <div className="w-32">
              <label className="lbl !mt-0">默认图片上限</label>
              <input className="field" type="number" value={defImg} onChange={(e) => setDefImg(Number(e.target.value))} />
            </div>
            <div className="w-28">
              <label className="lbl !mt-0">worker 并发</label>
              <input className="field" type="number" value={conc} onChange={(e) => setConc(Number(e.target.value))} />
            </div>
            <div className="flex-1 min-w-[160px]">
              <label className="lbl !mt-0">对外 API 密钥{settings.has_server_api_key ? '(已设置)' : ''}</label>
              <input className="field" type="password" placeholder="留空不改" value={serverKey} onChange={(e) => setServerKey(e.target.value)} />
            </div>
            <button className="btn btn-primary !py-2.5" onClick={saveSettings}>保存设置</button>
          </div>
          <p className="mt-2 text-[11px] text-slate-500">并发数变更将在服务重启后生效;日志条数立即生效。</p>
        </>
      )}
    </Modal>
  )
}

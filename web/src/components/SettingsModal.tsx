import { useEffect, useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import { api } from '../api'
import type { Config, Profile } from '../types'
import Modal from './Modal'

interface Props {
  open: boolean
  onClose: () => void
  config: Config
  profiles: Profile[]
  active: string
  onSaved: () => void
}

const NEW = '__new__'

export default function SettingsModal({ open, onClose, config, profiles, active, onSaved }: Props) {
  const [sel, setSel] = useState(active || NEW)
  const [form, setForm] = useState<Partial<Profile>>({})
  const [quality, setQuality] = useState('')
  const [timeout, setTimeoutV] = useState(300)
  const [concurrency, setConcurrency] = useState(3)
  const [serverKey, setServerKey] = useState('')
  const [oldPw, setOldPw] = useState('')
  const [newPw, setNewPw] = useState('')
  const [status, setStatus] = useState<{ msg: string; kind: 'ok' | 'err' } | null>(null)
  const isAdmin = config.role === 'admin'

  useEffect(() => {
    if (!open) return
    const init = active && profiles.find((p) => p.name === active) ? active : profiles[0]?.name || NEW
    setSel(init)
    loadProfile(init)
    setQuality(config.default_quality || '')
    setTimeoutV(config.timeout || 300)
    setConcurrency(config.concurrency || 3)
    setServerKey('')
    setStatus(null)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  function loadProfile(name: string) {
    if (name === NEW) {
      setForm({ name: '', base_url: '', api_key: '', model: 'gpt-image-2', reverse_model: 'gpt-4o', request_format: 'images' })
      return
    }
    const p = profiles.find((x) => x.name === name)
    if (p) setForm({ ...p, api_key: '' })
  }

  function onSelChange(name: string) {
    setSel(name)
    loadProfile(name)
  }

  async function del() {
    if (sel === NEW) return setStatus({ msg: '请先选中一个已保存的配置', kind: 'err' })
    if (!confirm(`删除配置「${sel}」?`)) return
    await api.deleteProfile(sel)
    setStatus({ msg: '已删除', kind: 'ok' })
    onSaved()
    setSel(NEW)
    loadProfile(NEW)
  }

  async function save() {
    const name = (form.name || '').trim()
    if (!name) return setStatus({ msg: '请填写配置名', kind: 'err' })
    try {
      await api.saveProfile({
        name,
        base_url: (form.base_url || '').trim(),
        api_key: (form.api_key || '').trim(),
        model: (form.model || '').trim(),
        reverse_model: (form.reverse_model || '').trim(),
        request_format: form.request_format || 'images',
      })
      await api.activateProfile(name)
      const cfgBody: Record<string, unknown> = { default_quality: quality.trim(), timeout: timeout || 300 }
      if (isAdmin) {
        cfgBody.concurrency = Math.max(1, Math.min(concurrency || 3, 16))
        if (serverKey.trim()) cfgBody.server_api_key = serverKey.trim()
      }
      await api.saveConfig(cfgBody)
      setStatus({ msg: '已保存 ✓ 当前配置:' + name, kind: 'ok' })
      onSaved()
      setTimeout(onClose, 600)
    } catch (e) {
      setStatus({ msg: (e as Error).message, kind: 'err' })
    }
  }

  async function changePwd() {
    if (!newPw.trim()) return setStatus({ msg: '请填写新密码', kind: 'err' })
    try {
      await api.changePassword(oldPw, newPw)
      setOldPw('')
      setNewPw('')
      setStatus({ msg: '密码已修改 ✓', kind: 'ok' })
    } catch (e) {
      setStatus({ msg: (e as Error).message, kind: 'err' })
    }
  }

  const curProfile = profiles.find((x) => x.name === sel)

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="设置"
      subtitle="可保存多套中转站,随时切换。密钥仅保存在服务端数据目录。"
      width={580}
      footer={
        <div className="flex items-center justify-between">
          <span className={`text-sm ${status?.kind === 'err' ? 'text-rose-400' : 'text-emerald-400'}`}>
            {status?.msg}
          </span>
          <div className="flex gap-2">
            <button className="btn btn-ghost" onClick={onClose}>取消</button>
            <button className="btn btn-primary" onClick={save}>保存</button>
          </div>
        </div>
      }
    >
      <label className="lbl !mt-0">中转站配置(可保存多套)</label>
      <div className="flex gap-2">
        <select className="field flex-1" value={sel} onChange={(e) => onSelChange(e.target.value)}>
          {profiles.map((p) => (
            <option key={p.name} value={p.name}>{p.name}</option>
          ))}
          <option value={NEW}>➕ 新建配置…</option>
        </select>
        <button className="btn btn-ghost shrink-0" onClick={() => onSelChange(NEW)}>
          <Plus size={15} /> 新建
        </button>
        <button className="btn btn-ghost shrink-0 hover:!border-rose-500 hover:!text-rose-400" onClick={del}>
          <Trash2 size={15} />
        </button>
      </div>

      <label className="lbl">配置名</label>
      <input className="field" value={form.name || ''} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="给这套配置起个名,如 中转站A" />

      <label className="lbl">中转站地址 Base URL</label>
      <input className="field" value={form.base_url || ''} onChange={(e) => setForm({ ...form, base_url: e.target.value })} placeholder="https://api.example.com(不含 /v1)" />

      <label className="lbl">API 密钥</label>
      <input
        className="field"
        type="password"
        value={form.api_key || ''}
        onChange={(e) => setForm({ ...form, api_key: e.target.value })}
        placeholder={curProfile?.has_api_key ? `已设置 ${curProfile.api_key}(留空=不修改)` : 'sk-...'}
      />

      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="lbl">默认生图模型</label>
          <input className="field" value={form.model || ''} onChange={(e) => setForm({ ...form, model: e.target.value })} placeholder="gpt-image-2" />
        </div>
        <div>
          <label className="lbl">反推用视觉模型</label>
          <input className="field" value={form.reverse_model || ''} onChange={(e) => setForm({ ...form, reverse_model: e.target.value })} placeholder="gpt-4o / gemini-2.5-flash" />
        </div>
      </div>

      <label className="lbl">默认请求格式</label>
      <select className="field" value={form.request_format || 'images'} onChange={(e) => setForm({ ...form, request_format: e.target.value })}>
        <option value="images">images(图片接口)</option>
        <option value="chat">chat(对话接口)</option>
      </select>

      <div className="mt-6 mb-3 sec-title">
        我的默认值<span className="ln" />
      </div>
      <div className={`grid gap-3 ${isAdmin ? 'grid-cols-3' : 'grid-cols-2'}`}>
        <div>
          <label className="lbl !mt-0">默认质量</label>
          <input className="field" value={quality} onChange={(e) => setQuality(e.target.value)} />
        </div>
        <div>
          <label className="lbl !mt-0">超时(秒)</label>
          <input className="field" type="number" value={timeout} onChange={(e) => setTimeoutV(+e.target.value)} />
        </div>
        {isAdmin && (
          <div>
            <label className="lbl !mt-0">队列并发数</label>
            <input className="field" type="number" min={1} max={16} value={concurrency} onChange={(e) => setConcurrency(+e.target.value)} />
          </div>
        )}
      </div>
      {isAdmin && (
        <>
          <p className="mt-2 text-[11px] text-slate-500">当前运行 {config.running_workers} 个 worker。改并发数后需重启服务生效。</p>
          <label className="lbl">对外 API 密钥(全局,可选)</label>
          <input
            className="field"
            type="password"
            value={serverKey}
            onChange={(e) => setServerKey(e.target.value)}
            placeholder={config.has_server_api_key ? '已设置(留空=不修改)' : '留空=不校验'}
          />
        </>
      )}

      <div className="mt-6 mb-3 sec-title">
        修改密码<span className="ln" />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <input className="field" type="password" value={oldPw} onChange={(e) => setOldPw(e.target.value)} placeholder="原密码" />
        <input className="field" type="password" value={newPw} onChange={(e) => setNewPw(e.target.value)} placeholder="新密码" />
      </div>
      <button className="btn btn-ghost mt-2 w-full" onClick={changePwd}>修改密码</button>
    </Modal>
  )
}

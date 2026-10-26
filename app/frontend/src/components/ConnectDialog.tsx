import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AddVPN, CheckVPN, Connect, ConnectVPN, Disconnect, Probe, SavedServers, VPNs } from '../../wailsjs/go/main/App'
import { main, remote, store } from '../../wailsjs/go/models'
import { useMobile } from '../mobile'
import { isRemote } from '../remote'
import { Icon } from './Icon'
import { Menu } from './Menu'
import { Modal } from './Modal'
import { VPN_TYPES, VpnEdit, VpnForm, type VpnCheck } from './VpnForm'

// vpn は通している VPN の名前。VPN なしなら空
export type Connected = { id: string; home: string; name: string; user: string; host: string; info: remote.Info; vpn?: string }

const input = 'field h-[34px] w-full px-2.5'
const label = 'mb-1.5 block text-[12px] font-medium text-soft'
const btn = 'btn'

type Step = 'wait' | 'run' | 'done' | 'fail'

// 保存済みのサーバーを入力欄の中身にする。パスワードは保存しないので空にする
const formOf = (s: store.Server) => ({ name: s.name, user: s.user, port: String(s.port), host: s.host, auth: s.auth || 'pass', password: '', keyFile: s.keyFile, vpn: s.vpn ?? '' })

// プローブの結果を「Linux · x86_64 · Ubuntu 24.04 · systemd · screen」の形にする
export function describeInfo(info: remote.Info, t: (k: string) => string) {
  const parts = [info.os, info.arch, info.distro]
  if (info.systemd) parts.push('systemd')
  if (info.docker) parts.push('Docker')
  if (info.screen) parts.push('screen')
  if (info.tmux) parts.push('tmux')
  if (!info.systemd && !info.docker) parts.push(t('probe.noSystemdDocker'))
  else if (!info.systemd) parts.push(t('probe.noSystemd'))
  else if (!info.docker) parts.push(t('probe.noDocker'))
  return parts.filter(Boolean).join(' · ')
}

// サーバーに接続するダイアログ。入力のあと「SSH で接続」「サーバーを確認」の2つの手順を見せる。
// VPN を通すときは、その前に「VPN につなぐ」を足す。初めてのサーバーでは、鍵の指紋を見せて確かめてもらう
export function ConnectDialog({ preset, onClose, onConnected }: {
  preset?: store.Server
  onClose: () => void
  onConnected: (c: Connected) => void
}) {
  const { t } = useTranslation()
  const mobile = useMobile()
  const [saved, setSaved] = useState<store.Server[]>([])
  // 保存済みから開いたときは、はじめからその中身を入れておく。
  // effect で入れると、入るより先にパスワードを打ったときに、打った側の古い中身で上書きされる
  const [f, setF] = useState(() => preset ? formOf(preset) : { name: '', user: '', port: '22', host: '', auth: 'pass', password: '', keyFile: '', vpn: '' })
  const [vpns, setVpns] = useState<main.VPNInfo[]>([])
  // 選んでいる保存済みの VPN は、右クリックで編集できる。スマホからは編集しない
  const [vpnMenu, setVpnMenu] = useState<{ x: number; y: number; vpn: main.VPNInfo } | null>(null)
  const [editing, setEditing] = useState<main.VPNInfo | null>(null)
  // 新しい VPN の入力と、それを読めたか
  const [newVpn, setNewVpn] = useState(new main.VPNInput({ name: '', type: 'wg', config: '', server: '', ca: '', group: '', psk: '', user: '', password: '' }))
  const [vpnCheck, setVpnCheck] = useState<VpnCheck>({ sum: null, error: '' })
  const [via, setVia] = useState<main.VPNInfo | null>(null)
  const [phase, setPhase] = useState<'form' | 'steps'>('form')
  const [vpnStep, setVpnStep] = useState<[Step, string]>(['wait', ''])
  const [ssh, setSsh] = useState<[Step, string]>(['wait', ''])
  const [probe, setProbe] = useState<[Step, string]>(['wait', ''])
  const [hostKey, setHostKey] = useState<main.HostKey | null>(null)
  const [done, setDone] = useState<Connected | null>(null)
  const passRef = useRef<HTMLInputElement>(null)
  const hostRef = useRef<HTMLInputElement>(null)
  const configRef = useRef<HTMLTextAreaElement>(null)
  const vpnUserRef = useRef<HTMLInputElement>(null)
  const aborted = useRef(false)

  const pick = (s: store.Server) => {
    setF(formOf(s))
    setTimeout(() => passRef.current?.focus())
  }

  useEffect(() => {
    SavedServers().then(list => setSaved(list ?? []))
    VPNs().then(list => setVpns(list ?? []))
    if (preset) passRef.current?.focus()
    else hostRef.current?.focus()
  }, [preset])

  const set = (k: keyof typeof f) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => setF({ ...f, [k]: e.target.value })
  const vpnName = (id?: string) => vpns.find(v => v.id === id)?.name ?? ''
  const name = f.name.trim() || f.host.trim()

  const vpnDetail = (v: main.VPNInfo, sum: { target: string; address: string }) =>
    t('vpn.detail', { type: t(`vpn.types.${v.type}`), target: sum.target, address: sum.address })

  const run = async (trust: string, v: main.VPNInfo | null) => {
    const host = f.host.trim(), user = f.user.trim(), port = parseInt(f.port, 10) || 22
    setPhase('steps')
    setHostKey(null)
    setVpnStep(['wait', ''])
    setSsh(['wait', ''])
    setProbe(['wait', ''])
    if (v) {
      setVpnStep(['run', ''])
      try {
        setVpnStep(['done', vpnDetail(v, await ConnectVPN(v.id, host))])
      } catch (err) {
        setVpnStep(['fail', String(err)])
        return
      }
    }
    setSsh(['run', ''])
    const r = await Connect(new main.ConnectRequest({ name, host, user, port, auth: f.auth, password: f.password, keyFile: f.keyFile.trim(), vpn: v?.id ?? '', trust }))
    if (r.hostKey) {
      setSsh(['fail', ''])
      setHostKey(r.hostKey)
      return
    }
    if (r.error) {
      setSsh(['fail', r.error])
      return
    }
    if (aborted.current) {
      Disconnect(r.id)
      return
    }
    setSsh(['done', `${user}@${host}:${port}`])
    setProbe(['run', ''])
    try {
      const info = await Probe(r.id)
      setProbe(['done', describeInfo(info, t)])
      setDone({ id: r.id, home: r.home, name, user, host, info, vpn: v?.name ?? '' })
    } catch (err) {
      setProbe(['fail', String(err)])
      Disconnect(r.id)
    }
  }

  const close = () => {
    aborted.current = true
    if (done) Disconnect(done.id)
    onClose()
  }

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!f.host.trim() || !f.user.trim()) return
    let v = vpns.find(x => x.id === f.vpn) ?? null
    if (f.vpn === 'new') {
      // 読めない設定のまま足さない。足した VPN は、接続できなくても次から選べる
      const file = VPN_TYPES.find(x => x.type === newVpn.type)?.file
      if (file && !newVpn.config.trim()) {
        setVpnCheck({ sum: null, error: t('vpn.needConfig') })
        configRef.current?.focus()
        return
      }
      let sum
      try {
        sum = await CheckVPN(newVpn)
      } catch (err) {
        setVpnCheck({ sum: null, error: String(err) })
        return
      }
      if (sum.login && (!newVpn.user.trim() || !newVpn.password)) {
        setVpnCheck({ sum, error: t('vpn.needLogin') })
        vpnUserRef.current?.focus()
        return
      }
      try {
        v = await AddVPN(newVpn)
      } catch (err) {
        setVpnCheck({ sum: null, error: String(err) })
        return
      }
      const added = v
      setVpns(list => [...list, added])
      setF(x => ({ ...x, vpn: added.id }))
    }
    setVia(v)
    run('', v)
  }

  if (phase === 'steps') {
    return (
      <Modal title={t('connect.stepsTitle', { name })} width={540} onClose={close}>
        <ol className="m-0 list-none p-0">
          {via && <StepRow state={vpnStep[0]} label={t('vpn.step', { name: via.name })} detail={vpnStep[1]} />}
          <StepRow state={ssh[0]} label={t('connect.stepSsh')} detail={ssh[1]} />
          {hostKey && <HostKeyCheck hostKey={hostKey} host={`${f.host}:${f.port}`} />}
          <StepRow state={probe[0]} label={t('connect.stepProbe')} detail={probe[1]} />
        </ol>
        <div className="mt-[18px] flex items-center gap-2">
          <div className="flex-1" />
          {done
            ? <button className="btn-primary" onClick={() => onConnected(done)} autoFocus>{t('connect.openServer')}</button>
            : <>
              {(vpnStep[0] === 'fail' || ssh[0] === 'fail' || probe[0] === 'fail') && <button className={btn} onClick={() => setPhase('form')}>{t('connect.back')}</button>}
              {hostKey && !hostKey.mismatch && <button className="btn-primary" onClick={() => run(hostKey.fingerprint, via)}>{t('connect.trust')}</button>}
              <button className={btn} onClick={close}>{t('connect.abort')}</button>
            </>}
        </div>
      </Modal>
    )
  }

  return (
    <Modal title={t('connect.title')} width={600} onClose={close}>
      <form onSubmit={submit}>
        {saved.length > 0 && <>
          <span className={label}>{t('connect.saved')}</span>
          <div className={`mb-5 grid gap-2.5 ${mobile ? 'grid-cols-1' : 'grid-cols-3'}`}>
            {saved.map(s => (
              <button
                type="button"
                key={s.id}
                className={`glass-card flex min-w-0 items-center gap-2.5 py-3 pr-3.5 pl-3 ${f.host === s.host && f.user === s.user && f.port === String(s.port) ? 'active' : ''}`}
                onClick={() => pick(s)}
              >
                <span className="saved-ic"><Icon name="server" /></span>
                <div className="min-w-0">
                  {/* VPN の名前が入りきらないときは、名前の下の行へ送る */}
                  <div className="flex min-w-0 flex-wrap items-center gap-x-2">
                    <span className="font-medium whitespace-nowrap text-strong">{s.name}</span>
                    {vpnName(s.vpn) && <span className="flex min-w-0 items-center gap-[3px] truncate text-[11px] text-muted" title={t('vpn.via', { name: vpnName(s.vpn) })} data-saved-vpn><Icon name="shield" size={12} />{vpnName(s.vpn)}</span>}
                  </div>
                  <div className="truncate font-mono text-[11px] text-muted">{s.user}@{s.host}</div>
                </div>
              </button>
            ))}
          </div>
        </>}
        <div className="grid grid-cols-3 gap-3.5">
          <label><span className={label}>{t('connect.name')}</span><input className={input} value={f.name} onChange={set('name')} /></label>
          <label><span className={label}>{t('connect.user')}</span><input className={input} value={f.user} onChange={set('user')} spellCheck={false} /></label>
          <label><span className={label}>{t('connect.port')}</span><input className={input} value={f.port} onChange={set('port')} inputMode="numeric" /></label>
          <label className="col-span-2"><span className={label}>{t('connect.host')}</span><input ref={hostRef} className={input} value={f.host} onChange={set('host')} spellCheck={false} /></label>
          <label>
            <span className={label}>{t('vpn.label')}</span>
            <select
              className={input} value={f.vpn} onChange={set('vpn')} data-vpn-select
              onContextMenu={e => {
                const v = vpns.find(x => x.id === f.vpn)
                if (!v || isRemote) return
                e.preventDefault()
                setVpnMenu({ x: e.clientX, y: e.clientY, vpn: v })
              }}
            >
              <option value="">{t('vpn.none')}</option>
              {vpns.map(v => <option key={v.id} value={v.id}>{v.name} ({t(`vpn.types.${v.type}`)})</option>)}
              <option value="new">{t('vpn.add')}</option>
            </select>
          </label>
          {f.vpn === 'new' && <VpnForm value={newVpn} onChange={setNewVpn} check={vpnCheck} onCheck={setVpnCheck} configRef={configRef} userRef={vpnUserRef} />}
          <label>
            <span className={label}>{t('connect.auth')}</span>
            <select className={input} value={f.auth} onChange={set('auth')}>
              <option value="pass">{t('connect.authPassword')}</option>
              <option value="key">{t('connect.authKey')}</option>
            </select>
          </label>
          {f.auth === 'key'
            ? <>
              <label className="col-span-2"><span className={label}>{t('connect.keyFile')}</span><input className={input} value={f.keyFile} onChange={set('keyFile')} spellCheck={false} /></label>
              <label className="col-span-3"><span className={label}>{t('connect.passphrase')}</span><input ref={passRef} className={input} type="password" value={f.password} onChange={set('password')} /></label>
            </>
            : <label className="col-span-2"><span className={label}>{t('connect.password')}</span><input ref={passRef} className={input} type="password" value={f.password} onChange={set('password')} /></label>}
        </div>
        <div className="mt-[22px] flex items-center gap-2">
          <div className="flex-1" />
          <button type="button" className={btn} onClick={close}>{t('connect.cancel')}</button>
          <button type="submit" className="btn-primary">{t('connect.submit')}</button>
        </div>
      </form>
      {vpnMenu && <Menu x={vpnMenu.x} y={vpnMenu.y} title={vpnMenu.vpn.name} onClose={() => setVpnMenu(null)} items={[
        { icon: 'edit', label: t('vpn.edit'), disabled: vpnMenu.vpn.inUse, fn: () => setEditing(vpnMenu.vpn) },
      ]} />}
      {editing && <VpnEdit vpn={editing} onClose={() => setEditing(null)} onSaved={() => VPNs().then(list => setVpns(list ?? []))} />}
    </Modal>
  )
}

function StepRow({ state, label: text, detail }: { state: Step; label: string; detail: string }) {
  // 回る弧はアクセントの光、終わった手順は弾んでチェックが出る。印は CSS の .step-ic で描く
  return (
    <li className={`grid grid-cols-[22px_minmax(0,1fr)] gap-3 py-2.5 transition-colors ${state === 'wait' ? 'text-faint' : 'text-strong'}`} data-step={state}>
      <span className={`step-ic mt-px ${state}`} />
      <div className="min-w-0">
        <div className="font-medium">{text}</div>
        {detail && <div className={`font-mono text-[12px] break-all ${state === 'fail' ? 'text-err' : 'text-muted'}`}>{detail}</div>}
      </div>
    </li>
  )
}

function HostKeyCheck({ hostKey, host }: { hostKey: main.HostKey; host: string }) {
  const { t } = useTranslation()
  return (
    <li className={`my-1 ml-[34px] list-none rounded-2xl p-4 ${hostKey.mismatch ? 'shadow-[inset_0_0_0_1px_var(--err)]' : 'bg-glass shadow-[var(--edge-glow)]'}`}>
      <div className={`mb-1.5 flex items-center gap-2 font-medium ${hostKey.mismatch ? 'text-err' : 'text-strong'}`}>
        <Icon name="lock" />{hostKey.mismatch ? t('connect.mismatchTitle') : t('connect.newKeyTitle')}
      </div>
      <p className="mt-0 mb-3 text-[12.5px] text-muted">{hostKey.mismatch ? t('connect.mismatchText') : t('connect.newKeyText')}</p>
      <div className="font-mono text-[12px] break-all">{host}</div>
      <div className="font-mono text-[12px] break-all" data-fingerprint>{hostKey.keyType} {hostKey.fingerprint}</div>
    </li>
  )
}

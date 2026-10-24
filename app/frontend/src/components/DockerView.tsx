import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Docker, DockerAction, DockerMounts, DockerStats, OpenShell } from '../../wailsjs/go/main/App'
import type { remote } from '../../wailsjs/go/models'
import { openFile } from '../openFile'
import { sendInput } from '../shellInput'
import { isDenied, requireSudo, sudoOn, useSudo, withSudo } from '../sudo'
import { toast } from '../toasts'
import { openStreamLog } from '../tools'
import { openWin } from '../windows'
import { Icon } from './Icon'
import { useFoot } from './Services'

const errText = (e: unknown) => String(e).replace(/^Error: /, '')
type Tab = 'containers' | 'images' | 'volumes'

// Docker のコンテナ・イメージ・ボリューム。docker を使う権限が無ければ sudo モードを聞く
export function DockerView({ serverId, home }: { serverId: string; home: string }) {
  const { t } = useTranslation()
  const sudo = useSudo(serverId)
  const [info, setInfo] = useState<remote.DockerInfo | null>(null)
  const [error, setError] = useState('')
  const [tab, setTab] = useState<Tab>('containers')
  const [open, setOpen] = useState<Record<string, boolean>>({})
  const [busy, setBusy] = useState<Record<string, boolean>>({})
  const [confirm, setConfirm] = useState('')
  const [stats, setStats] = useState<Record<string, remote.DockerStat>>({})
  const [pull, setPull] = useState('')
  const [say, foot] = useFoot()
  const tilde = (p: string) => p.startsWith(home + '/') ? '~' + p.slice(home.length) : p

  const load = (ask = false) => (ask ? withSudo(serverId, t('dk.needSudo'), () => Docker(serverId)) : Docker(serverId)).then(
    i => { setInfo(i); setError('') },
    e => setError(errText(e)),
  )
  useEffect(() => {
    load(true)
    const iv = setInterval(() => load(), 5000)
    return () => clearInterval(iv)
  }, [serverId, sudo])
  // docker stats は1回に1秒ほどかかるので、コンテナの一覧を見ている間だけ5秒おきに読む
  useEffect(() => {
    if (tab !== 'containers' || !info) return
    let alive = true
    const run = () => DockerStats(serverId).then(l => { if (alive) setStats(Object.fromEntries((l ?? []).map(s => [s.Name, s]))) }, () => {})
    run()
    const iv = setInterval(run, 5000)
    return () => {
      alive = false
      clearInterval(iv)
    }
  }, [serverId, tab, !!info, sudo])

  const act = (verb: string, target: string, done: string) => {
    setBusy(b => ({ ...b, [target]: true }))
    return withSudo(serverId, t('dk.needSudo'), () => DockerAction(serverId, verb, target)).then(
      () => say(done),
      e => say(isDenied(e) ? t('files.denied') : errText(e)),
    ).finally(() => {
      setBusy(b => ({ ...b, [target]: false }))
      load()
    })
  }
  const shell = async (name: string) => {
    const desk = document.querySelector<HTMLElement>(`[data-desktop="${serverId}"]`)?.getBoundingClientRect()
    // docker のグループに入っていない利用者は、sudo モードのときに sudo を付ける。パスワードはウィンドウの中で聞かれる
    const cmd = `${sudoOn(serverId) ? 'sudo ' : ''}docker exec -it ${name} sh`
    try {
      const id = await OpenShell(serverId, 100, 30, '')
      openWin({ serverId, kind: 'session', title: cmd, icon: 'box', shellId: id, detachable: false, replayFromIdle: false, rect: { w: 600, h: 340 } }, { w: desk?.width ?? 800, h: desk?.height ?? 500 })
      sendInput(id, cmd + '\r')
    } catch (e) {
      toast(errText(e), 'warn')
    }
  }

  if (!info) {
    if (error === 'このサーバーには Docker が入っていません') return <div className="tool-empty">{t('dk.none')}</div>
    if (error && isDenied(error)) {
      return (
        <div className="tool-empty">
          <div>
            <p className="mt-0">{t('dk.denied')}</p>
            <button className="sbtn primary" onClick={() => requireSudo(serverId, t('dk.needSudo'))}>{t('sudo.turnOn')}</button>
          </div>
        </div>
      )
    }
    return <div className="tool-empty">{error || t('mon.waiting')}</div>
  }

  const usedBy = (test: (c: remote.Container) => boolean) => info.containers.filter(test).map(c => c.Names)
  const chips = (names: string[]) => (
    <div className="dk-ports">{names.length ? names.map(n => <span key={n} className="chip-sm">{n}</span>) : <span className="text-[11.5px] text-muted">{t('dk.unused')}</span>}</div>
  )

  const containerRow = (c: remote.Container) => {
    const run = c.State === 'running', isOpen = !!open[c.Names], isBusy = !!busy[c.Names]
    const st = stats[c.Names]
    return (
      <div key={c.ID} className={`dk-item ${isOpen ? 'open' : ''}`} data-container={c.Names}>
        <div className="dk-row" onClick={e => { if (!(e.target as HTMLElement).closest('button')) setOpen(o => ({ ...o, [c.Names]: !isOpen })) }}>
          <span className={`text-faint transition-transform ${isOpen ? 'rotate-90' : ''}`}><Icon name="chevron" size={13} /></span>
          <span className={`svc-dot ${isBusy ? 'busy' : run ? 'on' : 'off'}`} />
          <div className="dk-name"><div className="font-mono text-[12.5px]">{c.Names}</div><div className="text-[11.5px] text-muted">{c.Image} · {c.Status}</div></div>
          <div className="dk-ports">{c.Ports.split(', ').filter(Boolean).map(p => <span key={p} className="chip-sm">{p}</span>)}</div>
          <span className="dk-stat">{run && st ? `CPU ${st.CPUPerc} · ${st.MemUsage.split(' / ')[0]}` : ''}</span>
          <div className="dk-acts">
            {confirm === c.Names
              ? <>
                <span className="text-[11.5px] text-muted">{t('dk.confirmRm')}</span>
                <button className="sbtn" onClick={() => setConfirm('')}>{t('dk.cancel')}</button>
                <button className="sbtn danger" onClick={() => { setConfirm(''); act('rm', c.Names, t('dk.removed', { name: c.Names })) }}>{t('dk.rm')}</button>
              </>
              : run
                ? <>
                  <button className="sbtn" onClick={() => shell(c.Names)}>{t('dk.shell')}</button>
                  <button className="sbtn" onClick={() => openStreamLog(serverId, 'docker', c.Names)}>{t('svc.log')}</button>
                  <button className="sbtn" disabled={isBusy} onClick={() => act('restart', c.Names, t('dk.restarted', { name: c.Names }))}>{t('svc.restart')}</button>
                  <button className="sbtn" disabled={isBusy} onClick={() => act('stop', c.Names, t('dk.stopped', { name: c.Names }))}>{t('svc.stop')}</button>
                </>
                : <>
                  <button className="sbtn" onClick={() => openStreamLog(serverId, 'docker', c.Names)}>{t('svc.log')}</button>
                  <button className="sbtn primary" disabled={isBusy} onClick={() => act('start', c.Names, t('dk.started', { name: c.Names }))}>{t('svc.start')}</button>
                  <button className="sbtn" disabled={isBusy} onClick={() => setConfirm(c.Names)}>{t('dk.rm')}</button>
                </>}
          </div>
        </div>
        {isOpen && <ContainerDetail serverId={serverId} c={c} tilde={tilde} onVolumes={() => setTab('volumes')} />}
      </div>
    )
  }

  const containers = () => {
    const groups = new Map<string, remote.Container[]>()
    for (const c of info.containers) groups.set(c.project, [...(groups.get(c.project) ?? []), c])
    if (!info.containers.length) return <div className="tool-empty">{t('dk.noContainers')}</div>
    return [...groups].map(([proj, cs]) => {
      const anyRun = cs.some(c => c.State === 'running')
      return (
        <div key={proj || '_'}>
          {proj
            ? (
              <div className="dk-group" data-project={proj}>
                <Icon name="layers" className="text-accent-2" /><span>{proj}</span>
                {cs[0].compose && <button className="mlink" onClick={() => openFile(serverId, { path: cs[0].compose.split(',')[0], name: cs[0].compose.split('/').pop()!, dir: false })}>{tilde(cs[0].compose.split(',')[0])}</button>}
                <div className="flex-1" />
                {anyRun && <button className="sbtn" onClick={() => cs.filter(c => c.State === 'running').forEach(c => act('restart', c.Names, t('dk.restarted', { name: c.Names })))}>{t('dk.restartAll')}</button>}
                {anyRun
                  ? <button className="sbtn" onClick={() => cs.filter(c => c.State === 'running').forEach(c => act('stop', c.Names, t('dk.stopped', { name: c.Names })))}>{t('dk.stopAll')}</button>
                  : <button className="sbtn primary" onClick={() => cs.forEach(c => act('start', c.Names, t('dk.started', { name: c.Names })))}>{t('dk.startAll')}</button>}
              </div>
            )
            : groups.size > 1 && <div className="dk-group">{t('dk.noCompose')}</div>}
          {cs.map(containerRow)}
        </div>
      )
    })
  }

  const doPull = () => {
    const v = pull.trim()
    if (!v) return
    setPull('')
    say(t('dk.pulling', { name: v }))
    act('pull', v, t('dk.pulled', { name: v }))
  }

  const images = () => info.images.map(i => {
    const ref = i.Tag && i.Tag !== '<none>' ? `${i.Repository}:${i.Tag}` : i.ID
    const used = usedBy(c => c.Image === ref || (i.Tag === 'latest' && c.Image === i.Repository))
    return (
      <div key={i.ID + ref} className="dk-row static" data-image={ref}>
        <span className="f-arc"><Icon name="archive" /></span>
        <div className="dk-name"><div className="font-mono text-[12.5px]">{ref}</div><div className="text-[11.5px] text-muted">{i.Size} · {i.ID.replace(/^sha256:/, '').slice(0, 12)}</div></div>
        {chips(used)}
        <div className="dk-acts"><button className="sbtn" disabled={used.length > 0 || !!busy[ref]} title={used.length ? t('dk.inUse') : undefined} onClick={() => act('rmi', ref, t('dk.removed', { name: ref }))}>{t('dk.rm')}</button></div>
      </div>
    )
  })

  const volumes = () => info.volumes.length ? info.volumes.map(v => {
    const used = usedBy(c => c.Mounts.split(',').includes(v.Name))
    return (
      <div key={v.Name} className="dk-row static" data-volume={v.Name}>
        <span className="f-code"><Icon name="db" /></span>
        <div className="dk-name"><div className="font-mono text-[12.5px]">{v.Name}</div><div className="font-mono text-[11.5px] text-muted">{v.Mountpoint}</div></div>
        {chips(used)}
        <div className="dk-acts"><button className="sbtn" disabled={used.length > 0 || !!busy[v.Name]} title={used.length ? t('dk.inUse') : undefined} onClick={() => act('volume-rm', v.Name, t('dk.removed', { name: v.Name }))}>{t('dk.rm')}</button></div>
      </div>
    )
  }) : <div className="tool-empty">{t('dk.noVolumes')}</div>

  return (
    <>
      <div className="win-toolbar">
        <div className="seg">
          {([['containers', info.containers.length], ['images', info.images.length], ['volumes', info.volumes.length]] as [Tab, number][]).map(([k, n]) => (
            <button key={k} className={k === tab ? 'active' : ''} onClick={() => setTab(k)}>{t(`dk.${k}`)} {n}</button>
          ))}
        </div>
        <div className="flex-1" />
        {tab === 'images' && <>
          <input className="mini-input" placeholder={t('dk.imageName')} spellCheck={false} value={pull} onChange={e => setPull(e.target.value)} onKeyDown={e => { if (e.key === 'Enter') doPull() }} />
          <button className="sbtn primary" onClick={doPull}>{t('dk.pull')}</button>
        </>}
      </div>
      <div className="dk-list">{tab === 'containers' ? containers() : tab === 'images' ? images() : volumes()}</div>
      {foot}
    </>
  )
}

function ContainerDetail({ serverId, c, tilde, onVolumes }: { serverId: string; c: remote.Container; tilde: (p: string) => string; onVolumes: () => void }) {
  const { t } = useTranslation()
  const [mounts, setMounts] = useState<remote.DockerMount[] | null>(null)
  useEffect(() => { DockerMounts(serverId, c.Names).then(m => setMounts(m ?? []), () => setMounts([])) }, [serverId, c.Names])
  const kv = (k: string, v: React.ReactNode) => <><span className="text-muted">{k}</span><div className="min-w-0">{v}</div></>
  return (
    <div className="dk-detail">
      {kv('ID', <span className="font-mono">{c.ID.slice(0, 12)}</span>)}
      {kv(t('dk.state'), c.Status)}
      {kv(t('dk.ports'), c.Ports || t('dk.none2'))}
      {kv(t('dk.mounts'), mounts === null ? '…' : mounts.length ? mounts.map(m => (
        <div key={m.Destination}>
          {m.Type === 'volume'
            ? <button className="mlink" onClick={onVolumes}>{m.Name}</button>
            : <span className="font-mono">{tilde(m.Source)}</span>}
          <span className="font-mono text-muted"> → {m.Destination}</span>
        </div>
      )) : t('dk.none2'))}
    </div>
  )
}

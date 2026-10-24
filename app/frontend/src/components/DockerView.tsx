import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import i18n from '../i18n'
import { Docker, DockerAction, DockerMounts, DockerStats, DockerVolumeSizes } from '../../wailsjs/go/main/App'
import type { remote } from '../../wailsjs/go/models'
import { openFile } from '../openFile'
import { useMobile } from '../mobile'
import { isDenied, requireSudo, useSudo, withSudo } from '../sudo'
import { openContainerShell, openStreamLog, revealInTree, takeDockerFocus } from '../tools'
import { Icon } from './Icon'
import { useFoot } from './Services'

const errText = (e: unknown) => String(e).replace(/^Error: /, '')

// docker ps の Status を、一覧に出す短い文と詳細に出す長い文にする。日本語の画面だけ訳し、ほかはそのまま出す。
// 形は moby@v27.5.1 container/state.go の State.String、時間は docker/go-units@v0.5.0 duration.go の HumanDuration
const UNITS: Record<string, string> = { second: '秒', minute: '分', hour: '時間', day: '日', week: '週間', month: 'か月', year: '年' }
const FIXED: Record<string, string> = { 'Less than a second': '1 秒未満', 'About a minute': '約 1 分', 'About an hour': '約 1 時間' }
const HEALTH: Record<string, string> = { Paused: '一時停止', healthy: '正常', unhealthy: '異常', 'health: starting': '確認中' }
export function statusText(status: string): { short: string; long: string } {
  if (i18n.language !== 'ja') return { short: status, long: status }
  const dur = (d: string) => {
    const m = /^(\d+) (second|minute|hour|day|week|month|year)s?$/.exec(d)
    return m ? `${m[1]} ${UNITS[m[2]]}` : FIXED[d] ?? d
  }
  let m = /^Up (.+?)(?: \((.+)\))?$/.exec(status)
  if (m) {
    const extra = m[2] ? ` · ${HEALTH[m[2]] ?? m[2]}` : ''
    return { short: `稼働 ${dur(m[1])}${extra}`, long: `稼働中 (${dur(m[1])}${extra})` }
  }
  m = /^(Exited|Restarting) \((-?\d+)\) (.+) ago$/.exec(status)
  if (m) {
    const ago = `${dur(m[3])}前`
    return m[1] === 'Exited'
      ? { short: `停止 ${ago}`, long: `停止 (終了コード ${m[2]} · ${ago})` }
      : { short: '再起動中', long: `再起動中 (終了コード ${m[2]} · ${ago})` }
  }
  const fixed = { Created: '作成済み', 'Removal In Progress': '削除中' }[status] ?? status
  return { short: fixed, long: fixed }
}
type Tab = 'containers' | 'images' | 'volumes'

// Docker のコンテナ・イメージ・ボリューム。docker を使う権限が無ければ sudo モードを聞く
export function DockerView({ serverId, home }: { serverId: string; home: string }) {
  const { t } = useTranslation()
  const sudo = useSudo(serverId)
  const mobile = useMobile()
  const [info, setInfo] = useState<remote.DockerInfo | null>(null)
  const [error, setError] = useState('')
  const [tab, setTab] = useState<Tab>('containers')
  const [open, setOpen] = useState<Record<string, boolean>>({})
  const [busy, setBusy] = useState<Record<string, boolean>>({})
  const [confirm, setConfirm] = useState('')
  const [stats, setStats] = useState<Record<string, remote.DockerStat>>({})
  const [pull, setPull] = useState('')
  // 取得しているイメージ。終わるまで一覧の頭に「取得中」の行を出す
  const [pulling, setPulling] = useState<string[]>([])
  // ボリュームの大きさ。docker system df で数えるので時間がかかる。ボリュームの一覧を開いたときに読む
  const [sizes, setSizes] = useState<Record<string, string>>({})
  useEffect(() => {
    if (tab === 'volumes') DockerVolumeSizes(serverId).then(s => setSizes(s ?? {}), () => {})
  }, [serverId, tab, sudo])
  const [say, foot] = useFoot()
  // リソース監視のプロセスの一覧から来たときに、開いて光らせるコンテナ
  const [focus, setFocus] = useState('')
  const [flash, setFlash] = useState('')
  useEffect(() => {
    const take = () => {
      const n = takeDockerFocus(serverId)
      if (!n) return
      setFocus(n)
      setTab('containers')
    }
    take()
    const on = (e: Event) => { if ((e as CustomEvent<string>).detail === serverId) take() }
    window.addEventListener('drag:docker-focus', on)
    return () => window.removeEventListener('drag:docker-focus', on)
  }, [serverId])
  useEffect(() => {
    const c = focus && info?.containers.find(x => x.Names === focus || x.ID.startsWith(focus))
    if (!c) return
    setFocus('')
    setOpen(o => ({ ...o, [c.Names]: true }))
    setFlash(c.Names)
    setTimeout(() => setFlash(''), 1200)
    requestAnimationFrame(() => document.querySelector(`[data-container="${CSS.escape(c.Names)}"]`)?.scrollIntoView({ block: 'nearest' }))
  }, [focus, info])
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
  const shell = (name: string) => openContainerShell(serverId, name)

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
      <div key={c.ID} className={`dk-item ${isOpen ? 'open' : ''} ${flash === c.Names ? 'flash' : ''}`} data-container={c.Names}>
        <div
          className="dk-row"
          onClick={e => { if (!(e.target as HTMLElement).closest('button')) setOpen(o => ({ ...o, [c.Names]: !isOpen })) }}
          // 動いているコンテナは、デスクトップへドラッグするとシェルを開く
          draggable={run && !mobile}
          title={run && !mobile ? t('dk.dragShell') : undefined}
          onDragStart={e => {
            e.dataTransfer.setData('application/x-drag-container', c.Names)
            e.dataTransfer.effectAllowed = 'copy'
          }}
        >
          <span className={`text-faint transition-transform ${isOpen ? 'rotate-90' : ''}`}><Icon name="chevron" size={13} /></span>
          <span className={`svc-dot ${isBusy ? 'busy' : run ? 'on' : 'off'}`} />
          <div className="dk-name"><div className="font-mono text-[12.5px]">{c.Names}</div><div className="text-[11.5px] text-muted">{c.Image} · {statusText(c.Status).short}</div></div>
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
    const full = /:[^/]+$/.test(v) || v.includes('@') ? v : `${v}:latest`
    if (info.images.some(i => `${i.Repository}:${i.Tag}` === full)) {
      say(t('dk.exists', { name: full }))
      return
    }
    setPull('')
    setPulling(p => [...p, full])
    act('pull', v, t('dk.pulled', { name: v })).finally(() => setPulling(p => p.filter(x => x !== full)))
  }

  const images = () => [...pulling.map(n => (
    <div key={'pull:' + n} className="dk-row static" data-pulling={n}>
      <span className="f-arc"><Icon name="archive" /></span>
      <div className="dk-name"><div className="font-mono text-[12.5px]">{n}</div><div className="text-[11.5px] text-muted">{t('dk.pullingRow')}</div></div>
      <div className="meter indet w-24 flex-none"><i /></div>
    </div>
  )), ...info.images.map(i => {
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
  })]

  const volumes = () => info.volumes.length ? info.volumes.map(v => {
    const used = usedBy(c => c.Mounts.split(',').includes(v.Name))
    return (
      <div key={v.Name} className="dk-row static" data-volume={v.Name}>
        <span className="f-code"><Icon name="db" /></span>
        <div className="dk-name"><div className="font-mono text-[12.5px]">{v.Name}</div><div className="font-mono text-[11.5px] text-muted">{sizes[v.Name] && sizes[v.Name] !== 'N/A' ? `${sizes[v.Name]} · ` : ''}{v.Mountpoint}</div></div>
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
      {kv(t('dk.state'), statusText(c.Status).long)}
      {kv(t('dk.ports'), c.Ports || t('dk.none2'))}
      {kv(t('dk.mounts'), mounts === null ? '…' : mounts.length ? mounts.map(m => (
        <div key={m.Destination}>
          {m.Type === 'volume'
            ? <button className="mlink" onClick={onVolumes}>{m.Name}</button>
            : <button className="mlink" title={t('files.reveal')} onClick={() => revealInTree(serverId, m.Source)}>{tilde(m.Source)}</button>}
          <span className="font-mono text-muted"> → {m.Destination}</span>
        </div>
      )) : t('dk.none2'))}
      {c.compose && kv(t('dk.compose'), (
        <button className="mlink" onClick={() => openFile(serverId, { path: c.compose.split(',')[0], name: c.compose.split(',')[0].split('/').pop()!, dir: false })}>{tilde(c.compose.split(',')[0])}</button>
      ))}
    </div>
  )
}

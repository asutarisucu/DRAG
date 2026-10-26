import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BigDirs, History, Mounts, Network, Processes } from '../../wailsjs/go/main/App'
import type { remote, store } from '../../wailsjs/go/models'
import { fmtGB, fmtRate, pct, useMetrics, type Metrics } from '../metrics'
import { fmtSize } from '../toasts'
import { openDockerAt, revealInTree } from '../tools'
import { requireSudo, useSudo } from '../sudo'
import { updateWin, type Win } from '../windows'
import { Icon } from './Icon'
import { Meter, Spark } from './Spark'

type View = 'cpu' | 'mem' | 'disk' | 'net'
const VIEWS: View[] = ['cpu', 'mem', 'disk', 'net']
type Range = 'live' | 'minute' | 'hour'

const last = (ms: Metrics[]) => ms.at(-1)

// 概要のカードと詳細の見出しに出す値。cpu・mem・disk は使用率、net は受信の速さ
function card(key: View, ms: Metrics[], t: (k: string, o?: Record<string, unknown>) => string) {
  const m = last(ms)
  switch (key) {
    case 'cpu':
      return { hist: ms.map(x => x.cpu), max: 100, val: `${(m?.cpu ?? 0).toFixed(0)}%`, sub: t('mon.cores', { n: m?.cores.length ?? 0 }) }
    case 'mem':
      return { hist: ms.map(x => pct(x.memUsed, x.memTotal)), max: 100, val: `${pct(m?.memUsed ?? 0, m?.memTotal ?? 0).toFixed(0)}%`, sub: `${fmtGB(m?.memUsed ?? 0)} / ${fmtGB(m?.memTotal ?? 0)} GB` }
    case 'disk':
      return { hist: ms.map(x => pct(x.diskUsed, x.diskTotal)), max: 100, val: `${pct(m?.diskUsed ?? 0, m?.diskTotal ?? 0).toFixed(0)}%`, sub: `${fmtGB(m?.diskUsed ?? 0)} / ${fmtGB(m?.diskTotal ?? 0)} GB` }
    case 'net': {
      const hist = ms.map(x => x.rx)
      return { hist, max: Math.max(200 * 1024, ...hist) * 1.15, val: fmtRate(m?.rx ?? 0), sub: t('mon.sent', { rate: fmtRate(m?.tx ?? 0) }) }
    }
  }
}

// 記録の値をグラフにする。Point は cpu・mem・disk が使用率、rx が受信の速さ
const pointValue = (key: View, p: store.Point) => key === 'cpu' ? p.cpu : key === 'mem' ? p.mem : key === 'disk' ? p.disk : p.rx

// リソース監視。概要のカードを押すと、その項目の詳細に替わる
export function MonitorView({ win, serverId, home }: { win: Win; serverId: string; home: string }) {
  const { t } = useTranslation()
  const ms = useMetrics(serverId)
  const view = (VIEWS as string[]).includes(win.view ?? '') ? win.view as View : null
  const show = (v: View | '') => updateWin(win.id, { view: v, ...(v && win.rect.h < 460 && !win.max ? { rect: { ...win.rect, h: 460, w: Math.max(win.rect.w, 600) } } : {}) })

  useEffect(() => {
    updateWin(win.id, { title: view ? `${t('tools.monitor')} · ${t(`mon.${view}`)}` : t('tools.monitor') })
  }, [view, t, win.id])

  if (!ms.length) return <div className="grid flex-1 place-items-center text-[12.5px] text-muted">{t('mon.waiting')}</div>
  if (!view) {
    return (
      <div className="mon" data-mon="overview">
        {VIEWS.map(k => {
          const c = card(k, ms, t)
          return (
            <button key={k} className="glass-card mon-card" data-card={k} title={t('mon.detailOf', { name: t(`mon.${k}`) })} onClick={() => show(k)}>
              <div className="mon-label">{t(`mon.${k}`)}<span className="mon-more">{t('mon.detail')}<Icon name="chevron" size={11} /></span></div>
              <div className="mon-value">{c.val}</div>
              <div className="mon-sub">{c.sub}</div>
              <Spark values={c.hist} max={c.max} />
            </button>
          )
        })}
      </div>
    )
  }
  return <Detail key={view} view={view} serverId={serverId} home={home} ms={ms} onShow={show} />
}

function Detail({ view, serverId, home, ms, onShow }: { view: View; serverId: string; home: string; ms: Metrics[]; onShow: (v: View | '') => void }) {
  const { t } = useTranslation()
  const [range, setRange] = useState<Range>('live')
  const [points, setPoints] = useState<store.Point[]>([])
  useEffect(() => {
    if (range === 'live') return
    History(serverId, range).then(p => setPoints(p ?? []))
  }, [serverId, range])
  const c = card(view, ms, t)
  const hist = range === 'live' ? c.hist : points.map(p => pointValue(view, p))
  const max = view === 'net' && range !== 'live' ? Math.max(200 * 1024, ...hist) * 1.15 : c.max
  return (
    <div className="flex min-h-0 flex-1 flex-col" data-mon={view}>
      <div className="mon-head">
        <button className="btn sm ghost !h-[26px]" onClick={() => onShow('')}>‹ {t('mon.overview')}</button>
        <div className="seg">
          {VIEWS.map(k => <button key={k} className={k === view ? 'active' : ''} onClick={() => onShow(k)}>{t(`mon.${k}`)}</button>)}
        </div>
      </div>
      <div className="mon-scroll">
        <div className="mon-top">
          <div>
            <div className="mon-value">{c.val}</div>
            <div className="mon-sub">{c.sub}</div>
            <div className="seg mt-2" data-range>
              {(['live', 'minute', 'hour'] as Range[]).map(r => <button key={r} className={r === range ? 'active' : ''} onClick={() => setRange(r)}>{t(`mon.range.${r}`)}</button>)}
            </div>
          </div>
          <div>
            <Spark values={hist} max={max} className="big" />
            {range !== 'live' && !hist.length && <div className="mon-sub">{t('mon.noHistory')}</div>}
          </div>
        </div>
        {view === 'cpu' && <CpuDetail serverId={serverId} m={ms.at(-1)!} />}
        {view === 'mem' && <MemDetail serverId={serverId} m={ms.at(-1)!} />}
        {view === 'disk' && <DiskDetail serverId={serverId} home={home} m={ms.at(-1)!} />}
        {view === 'net' && <NetDetail serverId={serverId} m={ms.at(-1)!} />}
      </div>
    </div>
  )
}

function Section({ title, children }: { title?: string; children: React.ReactNode }) {
  return <div className="mon-sec">{title && <h4>{title}</h4>}{children}</div>
}

// ms ごとに読み直す。窓を閉じたり詳細を替えたりしたら止める
function usePoll<T>(load: () => Promise<T>, ms: number, deps: unknown[]) {
  const [v, setV] = useState<T | null>(null)
  const [error, setError] = useState('')
  useEffect(() => {
    let alive = true
    const run = () => load().then(x => { if (alive) { setV(x); setError('') } }, e => { if (alive) setError(String(e)) })
    run()
    const iv = ms > 0 ? setInterval(run, ms) : 0
    return () => {
      alive = false
      clearInterval(iv)
    }
  }, deps)
  return [v, error] as const
}

function ProcTable({ serverId, sort, memTotal }: { serverId: string; sort: 'cpu' | 'rss'; memTotal: number }) {
  const { t } = useTranslation()
  const [list] = usePoll(() => Processes(serverId), 2000, [serverId])
  const top = [...(list ?? [])].sort((a, b) => sort === 'cpu' ? b.cpu - a.cpu || b.rss - a.rss : b.rss - a.rss).slice(0, 10)
  return (
    <div className="ptable" data-procs>
      <div className="prow phead"><span>{t('mon.proc')}</span><span>PID</span><span>{t('mon.user')}</span><span>CPU</span><span>{t('mon.mem')}</span></div>
      {top.map((p: remote.Proc) => (
        <div key={p.pid} className="prow">
          <div className="pname">
            <div className="font-mono text-[12.5px]">
              {p.name}
              {p.container && <button className="tag" title={t('mon.inDocker')} onClick={() => openDockerAt(serverId, p.container)}><Icon name="box" size={11} />{p.container}</button>}
            </div>
            <div className="pcmd" title={p.cmd}>{p.cmd}</div>
          </div>
          <span className="num">{p.pid}</span>
          <span className="truncate text-muted">{p.user}</span>
          <div className="pcell"><Meter value={p.cpu} /><span className="num">{p.cpu.toFixed(1)}%</span></div>
          <div className="pcell"><Meter value={pct(p.rss, memTotal)} alt /><span className="num">{fmtSize(p.rss)}</span></div>
        </div>
      ))}
    </div>
  )
}

function CpuDetail({ serverId, m }: { serverId: string; m: Metrics }) {
  const { t } = useTranslation()
  return <>
    <Section><ProcTable serverId={serverId} sort="cpu" memTotal={m.memTotal} /></Section>
    <Section title={t('mon.perCore')}>
      <div className="cores">
        {m.cores.map((v, i) => <div key={i} className="core"><span className="text-muted">{t('mon.core', { n: i })}</span><Meter value={v} /><span className="num">{v.toFixed(0)}%</span></div>)}
      </div>
      <div className="mon-sub">{t('mon.load', { load: m.load.map(x => x.toFixed(2)).join(' · ') })}</div>
    </Section>
  </>
}

function MemDetail({ serverId, m }: { serverId: string; m: Metrics }) {
  const { t } = useTranslation()
  const used = m.memUsed, cache = Math.min(m.memCache, m.memTotal - used), free = Math.max(0, m.memTotal - used - cache)
  const parts: [string, number, string][] = [[t('mon.memUsed'), used, 'c1'], [t('mon.memCache'), cache, 'c2'], [t('mon.memFree'), free, 'c3']]
  return <>
    <Section><ProcTable serverId={serverId} sort="rss" memTotal={m.memTotal} /></Section>
    <Section title={t('mon.breakdown')}>
      <div className="stack">{parts.map(([l, v, c]) => <i key={l} className={c} style={{ width: `${pct(v, m.memTotal)}%` }} />)}</div>
      <div className="legend">{parts.map(([l, v, c]) => <span key={l}><b className={c} />{l} {fmtGB(v)} GB</span>)}</div>
      <div className="mon-sub">{t('mon.swap', { used: fmtGB(m.swapUsed), total: fmtGB(m.swapTotal) })}</div>
    </Section>
  </>
}

function DiskDetail({ serverId, home, m }: { serverId: string; home: string; m: Metrics }) {
  const { t } = useTranslation()
  const sudo = useSudo(serverId)
  const [mounts] = usePoll(() => Mounts(serverId), 10000, [serverId])
  // 大きいフォルダを左クリックすると、そのフォルダの中を数える。opened は開いた順のフォルダで、空ならディスク全体
  const [opened, setOpened] = useState<string[]>([])
  const dir = opened.at(-1) ?? ''
  // du で数えるので時間がかかる。開いたときと、フォルダや sudo モードを替えたときに1回だけ数える
  const [big, dirsErr] = usePoll(() => BigDirs(serverId, dir).then(r => ({ ...r, dir })), 0, [serverId, sudo, dir])
  const cur = big?.dir === dir ? big : null
  const dirs = cur?.dirs ?? null
  const tilde = (p: string) => p === home ? '~' : p.startsWith(home + '/') ? '~' + p.slice(home.length) : p
  // フォルダの中では、開いたフォルダからの相対パスで出す。開いたフォルダのパスは上の行に出ている
  const label = (p: string) => dir && p.startsWith(dir + '/') ? p.slice(dir.length + 1) : tilde(p)
  const maxDir = dirs?.[0]?.size || 1
  return <>
    <Section title={t('mon.mounts')}>
      {(mounts ?? []).map(x => (
        <div key={x.path} className="mount"><span className="truncate font-mono" title={x.dev}>{x.path}</span><Meter value={pct(x.used, x.size)} /><span className="num">{fmtGB(x.used)} / {fmtGB(x.size)} GB</span></div>
      ))}
      <div className="mon-sub">{t('mon.io', { read: fmtRate(m.diskRead), write: fmtRate(m.diskWrite) })}</div>
    </Section>
    <Section>
      {dir && (
        <div className="mb-1.5 flex min-w-0 items-center gap-2" data-dir-nav>
          <button className="btn sm ghost !h-[26px] max-w-[45%] truncate" onClick={() => setOpened(o => o.slice(0, -1))}>
            ‹ {opened.length > 1 ? tilde(opened.at(-2)!) : t('mon.wholeDisk')}
          </button>
          <span className="min-w-0 truncate font-mono text-[12px] font-semibold">{tilde(dir)}</span>
        </div>
      )}
      {dirs === null && !dirsErr && <div className="mon-sub">{t('mon.counting')}</div>}
      {dirsErr && <div className="mon-sub">{dirsErr}</div>}
      {cur?.denied && (
        <div className="mon-sub flex items-center gap-2" data-dirs-denied>
          <span className="min-w-0 flex-1">{t('mon.dirsDenied')}</span>
          <button className="sbtn" onClick={() => requireSudo(serverId, t('mon.dirsNeedSudo'))}>{t('sudo.turnOn')}</button>
        </div>
      )}
      {dirs?.length === 0 && <div className="mon-sub">{t('mon.noDirs')}</div>}
      {dirs?.map(d => (
        <button key={d.path} className="dir-row" title={t('mon.dirHint')} onClick={() => setOpened(o => [...o, d.path])} onContextMenu={e => { e.preventDefault(); revealInTree(serverId, d.path, true) }}><span className="f-dir"><Icon name="folder" size={14} /></span><span className="truncate font-mono">{label(d.path)}</span><Meter value={d.size / maxDir * 100} alt /><span className="num">{fmtSize(d.size)}</span></button>
      ))}
    </Section>
  </>
}

function NetDetail({ serverId, m }: { serverId: string; m: Metrics }) {
  const { t } = useTranslation()
  const [net] = usePoll(() => Network(serverId), 3000, [serverId])
  return <>
    <Section title={t('mon.ifaces')}>
      {(net?.ifaces ?? []).map(i => <div key={i.name} className="mon-sub"><span className="font-mono">{i.name}</span>{i.addrs.length ? ` · ${i.addrs.join(', ')}` : ''}</div>)}
      <div className="mon-sub">{t('mon.rate', { rx: fmtRate(m.rx), tx: fmtRate(m.tx) })}</div>
    </Section>
    <Section title={t('mon.conns')}>
      <div className="ctable" data-conns>
        <div className="crow chead"><span>{t('mon.remote')}</span><span>{t('mon.localPort')}</span><span>{t('mon.proc')}</span></div>
        {(net?.conns ?? []).map((c, i) => <div key={i} className="crow"><span className="truncate font-mono">{c.remote}</span><span>{c.port}</span><span className="truncate font-mono text-muted">{c.proc || '—'}</span></div>)}
      </div>
    </Section>
    <Section title={t('mon.listen')}>
      <div className="flex flex-wrap gap-1.5" data-listen>{(net?.listen ?? []).map(l => <span key={`${l.addr}:${l.port}`} className="chip-sm">{l.port} {l.proc}</span>)}</div>
    </Section>
  </>
}


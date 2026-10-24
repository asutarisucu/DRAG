import { useSyncExternalStore } from 'react'
import { EventsOn } from '../wailsjs/runtime/runtime'

// サーバーの使用量。PC アプリが1秒ごとに "metrics" (番号, 値) で送ってくる。サーバーごとに直近60秒を持つ。
// 量はバイト、速さはバイト毎秒 (internal/remote/metrics.go の Metrics と同じ)
export type Metrics = {
  time: number
  cpu: number
  cores: number[]
  memTotal: number
  memUsed: number
  memCache: number
  swapTotal: number
  swapUsed: number
  rx: number
  tx: number
  diskRead: number
  diskWrite: number
  diskUsed: number
  diskTotal: number
  uptime: number
  load: number[]
}

export const KEEP = 60
const EMPTY: Metrics[] = []
const recent = new Map<string, Metrics[]>()
const listeners = new Set<() => void>()

EventsOn('metrics', (id: string, m: Metrics) => {
  recent.set(id, [...(recent.get(id) ?? []).slice(-(KEEP - 1)), m])
  listeners.forEach(l => l())
})

export const useMetrics = (serverId: string) => useSyncExternalStore(l => {
  listeners.add(l)
  return () => { listeners.delete(l) }
}, () => recent.get(serverId) ?? EMPTY)

export const pct = (used: number, total: number) => total > 0 ? used / total * 100 : 0

// 1024 で割って単位を付ける。ネットワークとディスクの速さに使う
export function fmtRate(b: number) {
  if (b < 1024) return `${Math.round(b)} B/s`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(b < 10 * 1024 ? 1 : 0)} KB/s`
  return `${(b / 1024 / 1024).toFixed(1)} MB/s`
}

export function fmtGB(b: number) {
  const g = b / 1024 ** 3
  return g < 10 ? g.toFixed(1) : g.toFixed(0)
}

// 稼働時間。mock/ の fmtUptime と同じ
export function fmtUptime(sec: number, t: (k: string, o?: Record<string, unknown>) => string) {
  const m = Math.floor(sec / 60), d = Math.floor(m / 1440), h = Math.floor(m % 1440 / 60), mi = m % 60
  return d ? t('mon.upDays', { d, h }) : h ? t('mon.upHours', { h, m: mi }) : t('mon.upMinutes', { m: mi })
}

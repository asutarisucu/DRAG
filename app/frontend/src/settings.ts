import { useSyncExternalStore } from 'react'
import i18n from './i18n'

// 設定と区画の大きさ。端末ごとに持つので、この画面の localStorage に置く
export type Settings = {
  lang: 'ja' | 'en'
  theme: 'dark' | 'light'
  accent: string // #rrggbb
  desk: 'gradient' | 'dots' | 'plain' // デスクトップの背景
  term: 'dark' | 'light' | 'solarized' // ターミナルの配色
  termSize: number
  sidebarW: number
  terminalH: number
  uploadConflict: 'ask' | 'overwrite' | 'keep' | 'skip'
  autoOn: boolean
  autoCmds: string[]
  snap: boolean
  followLog: boolean
  // 下の欄に出す項目
  sb: { uptime: boolean; cpu: boolean; mem: boolean; net: boolean }
}

export const DEFAULTS: Settings = { lang: 'ja', theme: 'dark', accent: '#5b9dff', desk: 'gradient', term: 'dark', termSize: 13, sidebarW: 260, terminalH: 230, uploadConflict: 'ask',
  autoOn: true, autoCmds: ['ssh', 'docker exec', 'docker attach', 'screen', 'tmux', 'mysql', 'psql'], snap: true, followLog: true,
  sb: { uptime: true, cpu: true, mem: true, net: true },
}
const KEY = 'drag-settings'

function load(): Settings {
  try {
    const saved = JSON.parse(localStorage.getItem(KEY) ?? '{}')
    return { ...DEFAULTS, ...saved, sb: { ...DEFAULTS.sb, ...saved.sb } }
  } catch {
    return { ...DEFAULTS }
  }
}

let current = load()
const listeners = new Set<() => void>()

function apply(s: Settings) {
  document.documentElement.dataset.theme = s.theme
  document.documentElement.dataset.desk = s.desk
  document.documentElement.dataset.term = s.term
  document.documentElement.style.setProperty('--accent', s.accent)
  document.documentElement.lang = s.lang
  if (i18n.language !== s.lang) i18n.changeLanguage(s.lang)
}
apply(current)

export function setSettings(patch: Partial<Settings>) {
  current = { ...current, ...patch }
  try {
    localStorage.setItem(KEY, JSON.stringify(current))
  } catch {
    // 書けなくても、この起動の間は効く
  }
  apply(current)
  listeners.forEach(l => l())
}

export function useSettings(): Settings {
  return useSyncExternalStore(l => {
    listeners.add(l)
    return () => listeners.delete(l)
  }, () => current)
}

import { useSyncExternalStore } from 'react'

// スマホの画面。幅が狭いと3区画を並べられないので、下のタブで1つずつ出す (mock/ と同じく 720px まで)
const query = matchMedia('(max-width: 720px)')
export const isMobile = () => query.matches
export const useMobile = () => useSyncExternalStore(l => {
  query.addEventListener('change', l)
  return () => query.removeEventListener('change', l)
}, () => query.matches)

export type MTab = 'files' | 'desktop' | 'terminal'
let tab: MTab = 'desktop'
const listeners = new Set<() => void>()

// ファイルを開いたらデスクトップへ、cd したらターミナルへのように、操作の先の区画へ移る。PC の画面では使わない
export function setMTab(t: MTab) {
  if (tab === t) return
  tab = t
  listeners.forEach(l => l())
}

export const getMTab = () => tab

export const useMTab = () => useSyncExternalStore(l => {
  listeners.add(l)
  return () => { listeners.delete(l) }
}, () => tab)

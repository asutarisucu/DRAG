// スマホの画面は、PC アプリが HTTPS で出す同じ画面を使い、Go の呼び出しを WebSocket で送る。
// wailsjs の関数は window.go と window.runtime を呼ぶので、Wails がいないときはその2つを WebSocket でまねる

type Listener = (...args: unknown[]) => void
type Pending = { resolve: (v: unknown) => void; reject: (e: unknown) => void }
type W = typeof window & { go?: unknown; runtime?: unknown }

// PC アプリの画面か、スマホのようにブラウザから開いた画面か
export let isRemote = false

// WebSocket の様子。切れたら画面に出し、つなぎ直したら読み込み直す
export type Link = 'ok' | 'lost'
let link: Link = 'ok'
const linkListeners = new Set<(l: Link) => void>()
export const onLink = (f: (l: Link) => void) => {
  linkListeners.add(f)
  return () => { linkListeners.delete(f) }
}
const setLink = (l: Link) => {
  link = l
  linkListeners.forEach(f => f(l))
}

function installRemote() {
  const w = window as W
  if (w.runtime) return
  isRemote = true
  let ws: WebSocket | null = null
  let next = 1
  let opened = false
  const pending = new Map<number, Pending>()
  const queue: string[] = []
  const listeners = new Map<string, Set<Listener>>()

  const connect = () => {
    ws = new WebSocket(`wss://${location.host}/api/ws`)
    ws.onopen = () => {
      // 切れている間のシェルの出力などは届いていないので、つなぎ直したら画面を作り直す
      if (opened) {
        location.reload()
        return
      }
      opened = true
      setLink('ok')
      for (const m of queue.splice(0)) ws!.send(m)
    }
    ws.onmessage = e => {
      const m = JSON.parse(e.data)
      if (m.ev) {
        listeners.get(m.ev)?.forEach(f => f(...m.a))
        return
      }
      const p = pending.get(m.id)
      pending.delete(m.id)
      if (m.e) p?.reject(m.e)
      else p?.resolve(m.r ?? undefined)
    }
    ws.onclose = () => {
      for (const p of pending.values()) p.reject('PC アプリとの接続が切れました')
      pending.clear()
      setLink('lost')
      setTimeout(connect, 2000)
    }
  }
  connect()

  const call = (method: string, args: unknown[]) => new Promise((resolve, reject) => {
    const id = next++
    pending.set(id, { resolve, reject })
    const msg = JSON.stringify({ id, m: method, a: args })
    if (ws?.readyState === WebSocket.OPEN) ws.send(msg)
    else queue.push(msg)
  })

  w.go = { main: { App: new Proxy({}, { get: (_, name) => (...args: unknown[]) => call(String(name), args) }) } }
  w.runtime = {
    EventsOnMultiple(name: string, cb: Listener, max: number) {
      let left = max
      const set = listeners.get(name) ?? new Set<Listener>()
      listeners.set(name, set)
      const f: Listener = (...a) => {
        if (left === 0) return
        if (left > 0 && --left === 0) set.delete(f)
        cb(...a)
      }
      set.add(f)
      return () => { set.delete(f) }
    },
    EventsOff(...names: string[]) {
      for (const n of names) listeners.delete(n)
    },
    ClipboardSetText: (text: string) => navigator.clipboard.writeText(text).then(() => true, () => false),
    // PC のファイルのドロップはスマホには無い
    OnFileDrop() {},
    OnFileDropOff() {},
  }
}

installRemote()

import { EventsOn } from '../wailsjs/runtime/runtime'

// シェルの出力は、OpenShell が番号を返すより先に届くことがある。
// 受け手が決まるまでの分は番号ごとにためておき、受け手が決まったら渡す。
// end はシェルの最初からの通算の位置で、その出力の末尾を指す
// 終わった印は { lost } で表す。lost は接続が切れて終わったとき
type Chunk = { data: Uint8Array; end: number } | { lost: boolean }
type Handler = { out: (data: Uint8Array) => void; exit: (lost: boolean) => void }

const handlers = new Map<string, { h: Handler; skipUntil: number }>()
const pending = new Map<string, Chunk[]>()
const foreground = new Map<string, string>()
const fgListeners = new Set<(id: string, cmdline: string) => void>()

function decode(b64: string): Uint8Array {
  const s = atob(b64)
  const b = new Uint8Array(s.length)
  for (let i = 0; i < s.length; i++) b[i] = s.charCodeAt(i)
  return b
}

// 渡し直した分と重なる出力は、重なっていない後ろだけを渡す
function give(e: { h: Handler; skipUntil: number }, c: Chunk) {
  if ('lost' in c) return e.h.exit(c.lost)
  const start = c.end - c.data.length
  if (c.end <= e.skipUntil) return
  e.h.out(start < e.skipUntil ? c.data.subarray(e.skipUntil - start) : c.data)
}

function deliver(id: string, c: Chunk) {
  const e = handlers.get(id)
  if (e) give(e, c)
  else pending.set(id, [...(pending.get(id) ?? []), c])
}

EventsOn('shell:out', (id: string, b64: string, end: number) => deliver(id, { data: decode(b64), end }))
EventsOn('shell:exit', (id: string, lost?: boolean) => {
  foreground.delete(id)
  deliver(id, { lost: !!lost })
})
EventsOn('shell:fg', (id: string, cmdline: string) => {
  foreground.set(id, cmdline)
  fgListeners.forEach(l => l(id, cmdline))
})

// 受け手を決める。skipUntil より前の出力は渡し直しで描いたものとして捨てる
export function attachShell(id: string, h: Handler, skipUntil = 0): () => void {
  const e = { h, skipUntil }
  handlers.set(id, e)
  for (const c of pending.get(id) ?? []) give(e, c)
  pending.delete(id)
  return () => {
    if (handlers.get(id) === e) handlers.delete(id)
  }
}

export function foregroundOf(id: string) {
  return foreground.get(id) ?? ''
}

export function onForeground(l: (id: string, cmdline: string) => void) {
  fgListeners.add(l)
  return () => { fgListeners.delete(l) }
}

export { decode }

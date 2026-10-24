import { EventsOn } from '../wailsjs/runtime/runtime'

// シェルの出力は、OpenShell が番号を返すより先に届くことがある。
// 受け手が決まるまでの分は番号ごとにためておき、受け手が決まったら渡す
type Handler = { out: (data: Uint8Array) => void; exit: () => void }

const handlers = new Map<string, Handler>()
const pending = new Map<string, (Uint8Array | null)[]>()

function decode(b64: string): Uint8Array {
  const s = atob(b64)
  const b = new Uint8Array(s.length)
  for (let i = 0; i < s.length; i++) b[i] = s.charCodeAt(i)
  return b
}

function deliver(id: string, chunk: Uint8Array | null) {
  const h = handlers.get(id)
  if (!h) {
    pending.set(id, [...(pending.get(id) ?? []), chunk])
    return
  }
  if (chunk) h.out(chunk)
  else h.exit()
}

EventsOn('shell:out', (id: string, b64: string) => deliver(id, decode(b64)))
// 終わった印は null で表す
EventsOn('shell:exit', (id: string) => deliver(id, null))

export function attachShell(id: string, h: Handler): () => void {
  handlers.set(id, h)
  for (const chunk of pending.get(id) ?? []) {
    if (chunk) h.out(chunk)
    else h.exit()
  }
  pending.delete(id)
  return () => handlers.delete(id)
}

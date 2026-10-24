import { useEffect, useRef, useState } from 'react'
import { ShellReplay, ShellResize } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { attachShell, decode } from '../shellBus'
import { sendInput } from '../shellInput'
import { useXterm, type TermSize } from './useXterm'

// ウィンドウの中のシェル。開くときに、残っている出力で画面を描き直してから続きを受け取る。
// shared はほかの端末で開いているシェル。1つのシェルは大きさを1つしか持てないので、
// その端末の大きさのまま描き、はみ出す分はスクロールで見る
export function ShellView({ shellId, fromIdle, fontSize, focused, shared, onExit }: {
  shellId: string
  fromIdle: boolean
  fontSize: number
  focused: boolean
  shared?: TermSize
  onExit: () => void
}) {
  const host = useRef<HTMLDivElement>(null)
  const [size, setSize] = useState(shared)
  const { termRef } = useXterm(host, fontSize, size)
  const sharedRef = useRef(!!shared)
  useEffect(() => {
    if (!shared) return
    return EventsOn('shell:size', (id: string, cols: number, rows: number) => { if (id === shellId) setSize({ cols, rows }) })
  }, [shellId, !!shared])
  const exitRef = useRef(onExit)
  exitRef.current = onExit

  useEffect(() => {
    const term = termRef.current!
    let detach = () => {}
    let disposed = false
    ShellReplay(shellId, fromIdle).then(r => {
      if (disposed) return
      term.write(decode(r.data))
      detach = attachShell(shellId, { out: d => term.write(d), exit: () => exitRef.current() }, r.end)
      // 移した先の大きさに合わせる。全画面のアプリはここで描き直す
      if (!sharedRef.current) ShellResize(shellId, term.cols, term.rows)
    }, () => exitRef.current())
    const onData = term.onData(d => sendInput(shellId, d))
    const onResize = term.onResize(({ cols, rows }) => { if (!sharedRef.current) ShellResize(shellId, cols, rows) })
    return () => {
      disposed = true
      detach()
      onData.dispose()
      onResize.dispose()
    }
  }, [shellId, fromIdle, termRef])

  useEffect(() => {
    if (focused) termRef.current?.focus()
  }, [focused, termRef])

  if (shared) {
    return (
      <div className="min-h-0 flex-1 overflow-auto bg-term" data-shared={shellId}>
        <div ref={host} className="inline-block" />
      </div>
    )
  }
  return <div ref={host} className="min-h-0 flex-1 bg-term" />
}

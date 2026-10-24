import { useEffect, useRef, useState } from 'react'
import { ShellReplay, ShellResize } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { attachShell, decode } from '../shellBus'
import { sendInput } from '../shellInput'
import { useXterm, type TermSize } from './useXterm'

// ウィンドウの中のシェル。開くときに、残っている出力で画面を描き直してから続きを受け取る。
// shared はほかの端末で開いているシェル。1つのシェルは大きさを1つしか持てないので、
// その端末の大きさのまま描き、はみ出す分はスクロールで見る。
// 接続が切れて終わったら lostText を出し、再接続して shellId が変わったら reopenText を出して続ける
export function ShellView({ shellId, fromIdle, fontSize, focused, shared, lostText, reopenText, onExit }: {
  shellId: string
  fromIdle: boolean
  fontSize: number
  focused: boolean
  shared?: TermSize
  lostText?: string
  reopenText?: string
  onExit: (lost: boolean) => void
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
  const texts = useRef({ lostText, reopenText })
  texts.current = { lostText, reopenText }
  const prevShell = useRef('')

  useEffect(() => {
    const term = termRef.current!
    let detach = () => {}
    let disposed = false
    const dim = (text?: string) => { if (text) term.write(`\r\n\x1b[2m${text}\x1b[0m\r\n`) }
    const reopened = prevShell.current !== '' && prevShell.current !== shellId
    prevShell.current = shellId
    ShellReplay(shellId, fromIdle).then(r => {
      if (disposed) return
      if (reopened) dim(texts.current.reopenText)
      term.write(decode(r.data))
      detach = attachShell(shellId, {
        out: d => term.write(d),
        exit: lost => {
          if (lost) dim(texts.current.lostText)
          exitRef.current(lost)
        },
      }, r.end)
      // 移した先の大きさに合わせる。全画面のアプリはここで描き直す
      if (!sharedRef.current) ShellResize(shellId, term.cols, term.rows)
    }, () => exitRef.current(false))
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

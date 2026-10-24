import { useEffect, useRef } from 'react'
import { ShellReplay, ShellResize } from '../../wailsjs/go/main/App'
import { attachShell, decode } from '../shellBus'
import { sendInput } from '../shellInput'
import { useXterm } from './useXterm'

// ウィンドウの中のシェル。開くときに、残っている出力で画面を描き直してから続きを受け取る
export function ShellView({ shellId, fromIdle, fontSize, focused, onExit }: {
  shellId: string
  fromIdle: boolean
  fontSize: number
  focused: boolean
  onExit: () => void
}) {
  const host = useRef<HTMLDivElement>(null)
  const { termRef } = useXterm(host, fontSize)
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
      ShellResize(shellId, term.cols, term.rows)
    }, () => exitRef.current())
    const onData = term.onData(d => sendInput(shellId, d))
    const onResize = term.onResize(({ cols, rows }) => ShellResize(shellId, cols, rows))
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

  return <div ref={host} className="min-h-0 flex-1 bg-term" />
}

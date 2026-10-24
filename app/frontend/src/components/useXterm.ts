import { useEffect, useRef } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

// xterm.js の画面を host に開き、host の大きさに合わせ続ける。
// 画面はこの hook の effect で作るので、使う側はこの hook より後に書いた effect の中で termRef を読む
export function useXterm(host: React.RefObject<HTMLDivElement | null>, fontSize: number) {
  const termRef = useRef<Terminal | null>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const sizeRef = useRef(fontSize)

  useEffect(() => {
    const term = new Terminal({
      fontFamily: '"Cascadia Mono", Consolas, "BIZ UDGothic", monospace',
      fontSize: sizeRef.current,
      lineHeight: 1.15,
      cursorBlink: true,
      scrollback: 5000,
      theme: { background: '#0a0d11', foreground: '#cfd6e0', cursor: '#5b9dff', selectionBackground: '#5b9dff55' },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(host.current!)
    fit.fit()
    termRef.current = term
    fitRef.current = fit
    const ro = new ResizeObserver(() => {
      // 隠れているときは大きさが測れないので合わせない
      if (host.current && host.current.offsetWidth > 0) fit.fit()
    })
    ro.observe(host.current!)
    return () => {
      ro.disconnect()
      term.dispose()
      termRef.current = null
      fitRef.current = null
    }
  }, [host])

  useEffect(() => {
    sizeRef.current = fontSize
    const term = termRef.current
    if (!term || term.options.fontSize === fontSize) return
    term.options.fontSize = fontSize
    fitRef.current?.fit()
  }, [fontSize])

  return { termRef, fitRef }
}

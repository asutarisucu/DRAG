import { useEffect, useRef } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

export type TermSize = { cols: number; rows: number }

// xterm.js の画面を host に開き、host の大きさに合わせ続ける。
// fixed を渡したときは合わせず、その桁数と行数で描く。ほかの端末で開いているシェルを、その端末の幅のまま見るときに使う。
// 画面はこの hook の effect で作るので、使う側はこの hook より後に書いた effect の中で termRef を読む
export function useXterm(host: React.RefObject<HTMLDivElement | null>, fontSize: number, fixed?: TermSize | null) {
  const termRef = useRef<Terminal | null>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const sizeRef = useRef(fontSize)
  const fixedRef = useRef(fixed)
  fixedRef.current = fixed

  useEffect(() => {
    const el = host.current!
    const term = new Terminal({
      fontFamily: '"Cascadia Mono", Consolas, "BIZ UDGothic", monospace',
      fontSize: sizeRef.current,
      lineHeight: 1.15,
      cursorBlink: true,
      scrollback: 5000,
      theme: { background: '#0a0d11', foreground: '#cfd6e0', cursor: '#5b9dff', selectionBackground: '#5b9dff55' },
      ...(fixedRef.current ?? {}),
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(el)
    if (!fixedRef.current) fit.fit()
    termRef.current = term
    fitRef.current = fit
    const ro = new ResizeObserver(() => {
      // 隠れているときは大きさが測れないので合わせない
      if (!fixedRef.current && el.offsetWidth > 0) fit.fit()
    })
    ro.observe(el)

    // xterm.js 6.0.0 は指でスクロールバックを動かさない。縦に動かしたときだけ受け取って行を送り、横は外側のスクロールに任せる
    let touch: { x: number; y: number; dir: '' | 'x' | 'y'; rest: number } | null = null
    const start = (e: TouchEvent) => {
      const t = e.touches[0]
      touch = { x: t.clientX, y: t.clientY, dir: '', rest: 0 }
    }
    const move = (e: TouchEvent) => {
      if (!touch || e.touches.length !== 1) return
      const t = e.touches[0]
      const dx = t.clientX - touch.x, dy = t.clientY - touch.y
      if (!touch.dir) {
        if (Math.abs(dx) < 8 && Math.abs(dy) < 8) return
        touch.dir = Math.abs(dy) > Math.abs(dx) ? 'y' : 'x'
      }
      if (touch.dir !== 'y') return
      e.preventDefault()
      const rowH = (el.querySelector('.xterm-screen')?.clientHeight ?? 0) / term.rows || 16
      touch.rest += touch.y - t.clientY
      const n = Math.trunc(touch.rest / rowH)
      if (n) {
        term.scrollLines(n)
        touch.rest -= n * rowH
      }
      touch.x = t.clientX
      touch.y = t.clientY
    }
    const end = () => { touch = null }
    el.addEventListener('touchstart', start, { passive: true })
    el.addEventListener('touchmove', move, { passive: false })
    el.addEventListener('touchend', end)
    return () => {
      ro.disconnect()
      el.removeEventListener('touchstart', start)
      el.removeEventListener('touchmove', move)
      el.removeEventListener('touchend', end)
      term.dispose()
      termRef.current = null
      fitRef.current = null
    }
  }, [host])

  useEffect(() => {
    if (fixed) termRef.current?.resize(fixed.cols, fixed.rows)
  }, [fixed?.cols, fixed?.rows])

  useEffect(() => {
    sizeRef.current = fontSize
    const term = termRef.current
    if (!term || term.options.fontSize === fontSize) return
    term.options.fontSize = fontSize
    if (!fixedRef.current) fitRef.current?.fit()
  }, [fontSize])

  return { termRef, fitRef }
}

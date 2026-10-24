import { useEffect, useRef } from 'react'
import { Terminal, type ITheme } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { useSettings, type Settings } from '../settings'

export type TermSize = { cols: number; rows: number }

// ターミナルの配色。背景と文字は style.css の --term-bg と --term-fg と同じ。ダークは 16 色を xterm.js のままにする
const SCHEMES: Record<Settings['term'], ITheme> = {
  dark: { background: '#0a0d11', foreground: '#cfd6e0' },
  light: {
    background: '#fbfbfd', foreground: '#2b3240',
    black: '#2b3240', red: '#c8302b', green: '#18875a', yellow: '#946400', blue: '#1f6feb', magenta: '#8250df', cyan: '#17809f', white: '#8a93a3',
    brightBlack: '#5c6779', brightRed: '#e5534b', brightGreen: '#1f9d6a', brightYellow: '#a86b00', brightBlue: '#3b82f6', brightMagenta: '#a371f7', brightCyan: '#1b95b8', brightWhite: '#b8c0cc',
  },
  solarized: {
    background: '#002b36', foreground: '#93a1a1',
    black: '#073642', red: '#dc322f', green: '#859900', yellow: '#b58900', blue: '#268bd2', magenta: '#d33682', cyan: '#2aa198', white: '#eee8d5',
    brightBlack: '#002b36', brightRed: '#cb4b16', brightGreen: '#586e75', brightYellow: '#657b83', brightBlue: '#839496', brightMagenta: '#6c71c4', brightCyan: '#93a1a1', brightWhite: '#fdf6e3',
  },
}
const themeOf = (s: Settings): ITheme => ({ ...SCHEMES[s.term], cursor: s.accent, selectionBackground: s.accent + '55' })

// xterm.js の画面を host に開き、host の大きさに合わせ続ける。
// fixed を渡したときは合わせず、その桁数と行数で描く。ほかの端末で開いているシェルを、その端末の幅のまま見るときに使う。
// 画面はこの hook の effect で作るので、使う側はこの hook より後に書いた effect の中で termRef を読む
export function useXterm(host: React.RefObject<HTMLDivElement | null>, fontSize: number, fixed?: TermSize | null) {
  const termRef = useRef<Terminal | null>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const sizeRef = useRef(fontSize)
  const fixedRef = useRef(fixed)
  fixedRef.current = fixed
  const settings = useSettings()
  const themeRef = useRef(themeOf(settings))

  useEffect(() => {
    const el = host.current!
    const term = new Terminal({
      fontFamily: '"Cascadia Mono", Consolas, "BIZ UDGothic", monospace',
      fontSize: sizeRef.current,
      lineHeight: 1.15,
      cursorBlink: true,
      scrollback: 5000,
      theme: themeRef.current,
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
    themeRef.current = themeOf(settings)
    if (termRef.current) termRef.current.options.theme = themeRef.current
  }, [settings.term, settings.accent])

  useEffect(() => {
    sizeRef.current = fontSize
    const term = termRef.current
    if (!term || term.options.fontSize === fontSize) return
    term.options.fontSize = fontSize
    if (!fixedRef.current) fitRef.current?.fit()
  }, [fontSize])

  return { termRef, fitRef }
}

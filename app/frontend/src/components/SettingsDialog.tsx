import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { DEFAULTS, setSettings, useSettings } from '../settings'
import { Icon, type IconName } from './Icon'
import { Modal } from './Modal'

function Seg<T extends string>({ value, options, onPick }: { value: T; options: [T, string][]; onPick: (v: T) => void }) {
  return (
    <div className="flex gap-0.5 rounded-lg border border-line bg-field p-0.5">
      {options.map(([v, label]) => (
        <button key={v} className={`h-6 rounded-md px-2.5 text-[12px] whitespace-nowrap ${v === value ? 'bg-raised text-text shadow-[0_0_0_1px_var(--border-strong)]' : 'text-muted hover:text-text'}`} onClick={() => onPick(v)}>{label}</button>
      ))}
    </div>
  )
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-[18px] border-b border-line py-[13px]">
      <div>{label}</div>
      {children}
    </div>
  )
}

// 設定。ページを替えても文字の大きさを変えても、窓の高さは変えない
export function SettingsDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  const s = useSettings()
  const [page, setPage] = useState<'look' | 'term' | 'ops'>('look')
  const pages: ['look' | 'term' | 'ops', IconName][] = [['look', 'window'], ['term', 'terminal'], ['ops', 'gear']]

  return (
    <Modal title={t('settings.title')} width={780} onClose={onClose}>
      <div className="grid h-[440px] flex-none grid-cols-[160px_minmax(0,1fr)] gap-6">
        <div className="flex flex-col gap-0.5">
          {pages.map(([k, icon]) => (
            <button key={k} className={`flex h-9 items-center gap-2.5 rounded-lg px-2.5 text-left ${page === k ? 'bg-accent-soft text-text' : 'text-muted hover:text-text'}`} onClick={() => setPage(k)}>
              <Icon name={icon} />{t(`settings.${k}`)}
            </button>
          ))}
          <div className="flex-1" />
          <button className="h-7 rounded-lg border border-line-strong bg-raised px-2.5 text-[12px] hover:bg-btn-hover" onClick={() => setSettings({ lang: DEFAULTS.lang, theme: DEFAULTS.theme, termSize: DEFAULTS.termSize, uploadConflict: DEFAULTS.uploadConflict })}>
            {t('settings.reset')}
          </button>
        </div>
        <div className="min-h-0 overflow-y-auto">
          <h3 className="mt-0 mb-1 text-[15px]">{t(`settings.${page}`)}</h3>
          {page === 'look' && <>
            <Row label={t('settings.lang')}>
              <Seg value={s.lang} options={[['ja', '日本語'], ['en', 'English']]} onPick={lang => setSettings({ lang })} />
            </Row>
            <Row label={t('settings.theme')}>
              <Seg value={s.theme} options={[['dark', t('settings.dark')], ['light', t('settings.light')]]} onPick={theme => setSettings({ theme })} />
            </Row>
          </>}
          {page === 'ops' && (
            <Row label={t('settings.uploadConflict')}>
              <select
                className="field h-8 min-w-[140px] rounded-lg border border-line-strong bg-field px-2.5 text-text outline-none"
                value={s.uploadConflict}
                onChange={e => setSettings({ uploadConflict: e.target.value as typeof s.uploadConflict })}
              >
                <option value="ask">{t('settings.conflictAsk')}</option>
                <option value="overwrite">{t('upload.overwrite')}</option>
                <option value="keep">{t('upload.keep')}</option>
                <option value="skip">{t('upload.skip')}</option>
              </select>
            </Row>
          )}
          {page === 'term' && (
            <Row label={t('settings.termSize')}>
              <div className="flex items-center gap-2.5">
                <input type="range" min={11} max={18} step={1} value={s.termSize} className="w-40 accent-[var(--accent)]" onChange={e => setSettings({ termSize: +e.target.value })} />
                <span className="font-mono text-[11.5px]">{s.termSize}px</span>
              </div>
            </Row>
          )}
        </div>
      </div>
    </Modal>
  )
}

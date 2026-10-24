import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import ja from './locales/ja'
import en from './locales/en'

// 既定は日本語。言語を足すときは locales/ にファイルを足してここに並べる
i18n.use(initReactI18next).init({
  resources: { ja: { translation: ja }, en: { translation: en } },
  lng: 'ja',
  fallbackLng: 'ja',
  interpolation: { escapeValue: false },
})

export default i18n

import React from 'react'
import ReactDOM from 'react-dom/client'
// テーマを先に読み込み、各コンポーネントの CSS がその後に来るようにする。
import './shared/theme.css'
import { App } from './app/App'

const rootEl = document.getElementById('root')
if (!rootEl) {
  throw new Error('root element not found')
}
if ('serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch((err) => {
      console.warn('service worker登録に失敗しました', err)
    })
  })
}
ReactDOM.createRoot(rootEl).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)

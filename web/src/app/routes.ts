import { useCallback, useEffect, useState } from 'react'

// Screen は画面の種類。live はメイン画面。
export type Screen = 'live' | 'eventlog' | 'memory' | 'scenes' | 'jobs' | 'settings'

// 画面と URL の対応。画面は ?ui=<ui> で開く。古いパス（/admin など）で開いた場合も受け付ける。
// 画面を足すときは、ここに 1 行足す。
const routes: { screen: Exclude<Screen, 'live'>; ui: string; path: string }[] = [
  { screen: 'eventlog', ui: 'admin', path: '/admin' },
  { screen: 'memory', ui: 'memory', path: '/memory' },
  { screen: 'scenes', ui: 'scenes', path: '/scenes' },
  { screen: 'jobs', ui: 'jobs', path: '/jobs' },
  { screen: 'settings', ui: 'settings', path: '/settings' },
]

// screenFromLocation は URL のクエリとパスから画面を決める。?ui= をパスより優先する。
export function screenFromLocation(search: string, pathname: string): Screen {
  const ui = new URLSearchParams(search).get('ui')
  return routes.find(route => route.ui === ui)?.screen
    ?? routes.find(route => pathname.startsWith(route.path))?.screen
    ?? 'live'
}

// screenURL は画面を開く URL を返す。ui 以外のクエリはそのまま残す。
export function screenURL(screen: Screen, search: string): string {
  const params = new URLSearchParams(search)
  const ui = routes.find(route => route.screen === screen)?.ui
  if (ui) {
    params.set('ui', ui)
  } else {
    params.delete('ui')
  }
  const query = params.toString()
  return query ? `/?${query}` : '/'
}

// useScreen は今の画面と、画面を切り替える navigate を返す。ブラウザの戻る・進むにも追従する。
export function useScreen(): [Screen, (screen: Screen) => void] {
  const [screen, setScreen] = useState<Screen>(() => screenFromLocation(window.location.search, window.location.pathname))
  useEffect(() => {
    const onPopState = () => setScreen(screenFromLocation(window.location.search, window.location.pathname))
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [])
  const navigate = useCallback((next: Screen) => {
    window.history.pushState({}, '', screenURL(next, window.location.search))
    setScreen(next)
  }, [])
  return [screen, navigate]
}

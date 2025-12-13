const logEl = document.getElementById('log') as HTMLElement | null

export function log(msg: string) {
  if (!logEl) return
  const time = new Date().toISOString()
  logEl.textContent += `[${time}] ${msg}\n`
  logEl.scrollTop = logEl.scrollHeight
}

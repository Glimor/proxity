// Byte/duration formatting helpers (locale-agnostic units).

export function fmtBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) n = 0
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024
    i++
  }
  const v = i === 0 ? Math.round(n).toString() : n.toFixed(n >= 100 ? 0 : n >= 10 ? 1 : 2)
  return `${v} ${units[i]}`
}

export function fmtBps(n: number): string {
  return fmtBytes(n)
}

export function fmtDurationMs(ms: number): { n: number; unit: 's' | 'm' | 'h' | 'd' } {
  const s = Math.max(0, Math.round(ms / 1000))
  if (s < 60) return { n: s, unit: 's' }
  const m = Math.floor(s / 60)
  if (m < 60) return { n: m, unit: 'm' }
  const h = Math.floor(m / 60)
  if (h < 24) return { n: h, unit: 'h' }
  return { n: Math.floor(h / 24), unit: 'd' }
}

export function fmtSeconds(sec: number): { n: number; unit: 's' | 'm' | 'h' | 'd' } {
  return fmtDurationMs(sec * 1000)
}

export function exeName(path: string): string {
  if (!path) return ''
  const i = Math.max(path.lastIndexOf('\\'), path.lastIndexOf('/'))
  return i >= 0 ? path.slice(i + 1) : path
}

export function fmtTime(iso: string): string {
  const d = new Date(iso)
  if (isNaN(d.getTime())) return iso
  return d.toLocaleString()
}

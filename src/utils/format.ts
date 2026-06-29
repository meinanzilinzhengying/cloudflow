export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

export function formatNumber(num: number): string {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return num.toString()
}

export function formatTime(ts: string): string {
  const d = new Date(ts)
  return d.toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).replace(/\//g, '-')
}

export function statusColor(status: string): string {
  switch (status) {
    case 'online': return 'success'
    case 'degraded': return 'warning'
    case 'offline': return 'danger'
    case 'high': return 'danger'
    case 'medium': return 'warning'
    case 'low': return 'info'
    case 'pending': return 'danger'
    case 'handled': return 'success'
    case 'ignored': return 'info'
    default: return ''
  }
}

export function statusText(status: string): string {
  const map: Record<string, string> = {
    online: '在线', offline: '离线', degraded: '降级',
    high: '高危', medium: '中危', low: '低危',
    pending: '未处理', handled: '已处理', ignored: '已忽略',
  }
  return map[status] || status
}

/**
 * queryClickHouse
 * 通过 nginx 代理 /api/clickhouse/ 直连 ClickHouse HTTP 接口
 * 自动追加 FORMAT JSONEachRow，返回解析后的行数组
 */
export async function queryClickHouse(sql: string): Promise<any[]> {
  try {
    // ClickHouse HTTP 接口：GET /?query=SQL 或 POST body
    // 这里用 GET + query 参数（避免 405）
    const base = '/api/clickhouse/'
    const url = base + '?query=' + encodeURIComponent(sql.trim() + ' FORMAT JSONEachRow')
    const resp = await fetch(url, { method: 'GET' })
    if (!resp.ok) {
      const err = await resp.text()
      console.error('[ClickHouse] 查询失败:', err)
      return []
    }
    const text = (await resp.text()).trim()
    if (!text) return []
    return text
      .split('\n')
      .filter(l => l.trim())
      .map(l => { try { return JSON.parse(l) } catch { return null } })
      .filter(Boolean)
  } catch (e) {
    console.error('[ClickHouse] 请求异常:', e)
    return []
  }
}

/**
 * PROBE_FILTER
 * 追加到 WHERE 子句，用于按 probe_id 过滤
 * 机顶盒场景：如需限定探针，取消注释并填写实际 probe_id
 * 例: " AND probe_id = 'stb-001'"
 */
export const PROBE_FILTER = ''

export type StbLogRow = {
  timestamp: string
  src_ip: string
  dst_ip: string
  protocol: string
  dst_port: number
  bytes: number
  packets: number
  details: string
  event_type: string
}

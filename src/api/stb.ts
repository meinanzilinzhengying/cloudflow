import axios from 'axios'

// Use POST instead of GET to avoid proxy/firewall issues with long URLs
const CLICKHOUSE_BASE = '/api/clickhouse'

// Remove the old function and replace with a simpler approach
async function queryClickHouse(sql: string): Promise<string> {
  const res = await axios.post(CLICKHOUSE_BASE, 'query=' + encodeURIComponent(sql), {
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    timeout: 30000
  })
  return res.data
}

function parseTSV(tsv: string): string[][] {
  const lines = tsv.trim().split('\n')
  return lines.filter(l => l.trim()).map(l => l.split('\t'))
}

export interface OverviewMetrics {
  totalEvents: number
  totalMB: number
  totalPackets: number
  collectRate: number
  activeProtocols: number
  uniqueDstIPs: number
  lastHeartbeat: string
}

export interface ProtocolStat {
  protocol: string
  packets: number
  bytes: number
  percentage: number
}

export interface TopTalker {
  dstIp: string
  protocol: string
  bytes: number
  mb: number
  packets: number
  events: number
}

export interface TrafficTrend {
  time: string
  mbps: number
  pps: number
}

export interface IPTVChannel {
  multicastAddr: string
  channelPort: number
  totalMB: number
  pktCount: number
}

// === 机顶盒总览 ===
export async function getOverviewMetrics(): Promise<OverviewMetrics> {
  const [eventsRes, trafficRes, protocolRes, ipRes, heartbeatRes] = await Promise.all([
    queryClickHouse("SELECT count() AS total, round(count() / 60, 1) AS rate FROM cloudflow.ebpf_events WHERE probe_id = 'stb-188-bpf' AND timestamp > now() - INTERVAL 24 HOUR"),
    queryClickHouse("SELECT round(sum(bytes) / 1048576, 2) AS total_mb, sum(packets) AS total_packets FROM cloudflow.ebpf_events WHERE probe_id = 'stb-188-bpf' AND timestamp > now() - INTERVAL 24 HOUR"),
    queryClickHouse("SELECT count(DISTINCT protocol) AS cnt FROM cloudflow.ebpf_events WHERE probe_id = 'stb-188-bpf' AND timestamp > now() - INTERVAL 24 HOUR"),
    queryClickHouse("SELECT count(DISTINCT dst_ip) AS cnt FROM cloudflow.ebpf_events WHERE probe_id = 'stb-188-bpf' AND timestamp > now() - INTERVAL 24 HOUR"),
    queryClickHouse("SELECT formatDateTime(max(timestamp), '%Y-%m-%d %H:%i:%s') AS last FROM cloudflow.ebpf_events WHERE probe_id = 'stb-188-bpf'"),
  ])
  const e = parseTSV(eventsRes)[0]
  const t = parseTSV(trafficRes)[0]
  const p = parseTSV(protocolRes)[0]
  const ip = parseTSV(ipRes)[0]
  const hb = parseTSV(heartbeatRes)[0]
  return {
    totalEvents: parseInt(e?.[0] || '0'),
    collectRate: parseFloat(e?.[1] || '0'),
    totalMB: parseFloat(t?.[0] || '0'),
    totalPackets: parseInt(t?.[1] || '0'),
    activeProtocols: parseInt(p?.[0] || '0'),
    uniqueDstIPs: parseInt(ip?.[0] || '0'),
    lastHeartbeat: hb?.[0] || '无数据',
  }
}

export async function getProtocolDistribution(): Promise<ProtocolStat[]> {
  const res = await queryClickHouse("SELECT protocol, sum(packets) AS pkt, sum(bytes) AS bytes, round(sum(packets) * 100.0 / nullif(sum(sum(packets)) OVER (), 0), 1) AS pct FROM cloudflow.ebpf_events WHERE probe_id = 'stb-188-bpf' AND category = 'network' AND timestamp > now() - INTERVAL 24 HOUR GROUP BY protocol ORDER BY bytes DESC")
  return parseTSV(res).map(r => ({
    protocol: r[0], packets: parseInt(r[1] || '0'), bytes: parseInt(r[2] || '0'), percentage: parseFloat(r[3] || '0')
  }))
}

export async function getTopTalkers(limit = 10): Promise<TopTalker[]> {
  const res = await queryClickHouse("SELECT dst_ip, protocol, sum(bytes) AS total_bytes, round(sum(bytes) / 1048576, 2) AS total_mb, sum(packets) AS packet_count, count() AS event_count FROM cloudflow.ebpf_events WHERE probe_id = 'stb-188-bpf' AND category = 'network' AND timestamp > now() - INTERVAL 24 HOUR GROUP BY dst_ip, protocol ORDER BY total_bytes DESC LIMIT " + limit)
  return parseTSV(res).map(r => ({
    dstIp: r[0], protocol: r[1], bytes: parseInt(r[2] || '0'), mb: parseFloat(r[3] || '0'), packets: parseInt(r[4] || '0'), events: parseInt(r[5] || '0')
  }))
}

export async function getTrafficTrend(): Promise<TrafficTrend[]> {
  const res = await queryClickHouse("SELECT formatDateTime(toStartOfInterval(timestamp, INTERVAL 1 MINUTE), '%H:%i') AS t, round(sum(bytes) * 8 / 1048576 / 60, 3) AS mbps, round(sum(packets) / 60, 1) AS pps FROM cloudflow.ebpf_events WHERE probe_id = 'stb-188-bpf' AND category = 'network' AND timestamp > now() - INTERVAL 24 HOUR GROUP BY toStartOfInterval(timestamp, INTERVAL 1 MINUTE) ORDER BY toStartOfInterval(timestamp, INTERVAL 1 MINUTE)")
  return parseTSV(res).map(r => ({ time: r[0], mbps: parseFloat(r[1] || '0'), pps: parseFloat(r[2] || '0') }))
}

// === 流量分布明细 ===
export async function getTrafficSummary(): Promise<{ totalMB: number; totalPackets: number; tcpBytes: number; udpBytes: number }> {
  const res = await queryClickHouse("SELECT round(sum(bytes) / 1048576, 2) AS total_mb, sum(packets) AS total_packets, round(sum(if(protocol = 'TCP', bytes, 0)) / 1048576, 2) AS tcp_mb, round(sum(if(protocol = 'UDP', bytes, 0)) / 1048576, 2) AS udp_mb FROM cloudflow.ebpf_events WHERE probe_id = 'stb-188-bpf' AND category = 'network' AND timestamp > now() - INTERVAL 24 HOUR")
  const r = parseTSV(res)[0]
  return { totalMB: parseFloat(r?.[0] || '0'), totalPackets: parseInt(r?.[1] || '0'), tcpBytes: parseFloat(r?.[2] || '0'), udpBytes: parseFloat(r?.[3] || '0') }
}

// === IPTV质量（基于UDP多播分析） ===
export async function getIPTVChannels(): Promise<IPTVChannel[]> {
  const res = await queryClickHouse("SELECT dst_ip, dst_port, round(sum(bytes) / 1048576, 2) AS total_mb, count() AS pkt_cnt FROM cloudflow.ebpf_events WHERE probe_id = 'stb-188-bpf' AND category = 'network' AND protocol = 'UDP' AND (dst_ip LIKE '224.%' OR dst_ip LIKE '225.%' OR dst_ip LIKE '239.%') AND timestamp > now() - INTERVAL 24 HOUR GROUP BY dst_ip, dst_port ORDER BY total_mb DESC LIMIT 20")
  return parseTSV(res).map(r => ({ multicastAddr: r[0], channelPort: parseInt(r[1] || '0'), totalMB: parseFloat(r[2] || '0'), pktCount: parseInt(r[3] || '0') }))
}

export async function getUDPTrafficTrend(): Promise<TrafficTrend[]> {
  const res = await queryClickHouse("SELECT formatDateTime(toStartOfInterval(timestamp, INTERVAL 1 MINUTE), '%H:%i') AS t, round(sum(bytes) * 8 / 1048576 / 60, 3) AS mbps, round(sum(packets) / 60, 1) AS pps FROM cloudflow.ebpf_events WHERE probe_id = 'stb-188-bpf' AND protocol = 'UDP' AND timestamp > now() - INTERVAL 24 HOUR GROUP BY toStartOfInterval(timestamp, INTERVAL 1 MINUTE) ORDER BY toStartOfInterval(timestamp, INTERVAL 1 MINUTE)")
  return parseTSV(res).map(r => ({ time: r[0], mbps: parseFloat(r[1] || '0'), pps: parseFloat(r[2] || '0') }))
}

// === 换台行为分析（基于HTTP请求） ===
export async function getHTTPChannelChanges(): Promise<{ time: string; count: number }[]> {
  const res = await queryClickHouse("SELECT formatDateTime(toStartOfInterval(timestamp, INTERVAL 5 MINUTE), '%H:%i') AS t, count() AS cnt FROM cloudflow.ebpf_events WHERE probe_id = 'stb-188-bpf' AND event_type = 'http' AND timestamp > now() - INTERVAL 24 HOUR GROUP BY toStartOfInterval(timestamp, INTERVAL 5 MINUTE) ORDER BY toStartOfInterval(timestamp, INTERVAL 5 MINUTE)")
  return parseTSV(res).map(r => ({ time: r[0], count: parseInt(r[1] || '0') }))
}

export async function getActiveConnections(): Promise<{ dstIp: string; port: number; bytes: number; packets: number }[]> {
  const res = await queryClickHouse("SELECT dst_ip, dst_port, sum(bytes) AS total_bytes, sum(packets) AS total_packets FROM cloudflow.ebpf_events WHERE probe_id = 'stb-188-bpf' AND timestamp > now() - INTERVAL 24 HOUR GROUP BY dst_ip, dst_port ORDER BY total_bytes DESC LIMIT 20")
  return parseTSV(res).map(r => ({ dstIp: r[0], port: parseInt(r[1] || '0'), bytes: parseInt(r[2] || '0'), packets: parseInt(r[3] || '0') }))
}

export async function getMetricsHistory(): Promise<{ time: string; cpu: number; mem: number; netRx: number; netTx: number }[]> {
  const res = await queryClickHouse("SELECT formatDateTime(toStartOfInterval(timestamp, INTERVAL 1 MINUTE), '%H:%i') AS t, round(avg(JSONExtractFloat(details, 'cpu_percent')), 1) AS cpu, round(avg(JSONExtractFloat(details, 'memory_percent')), 1) AS mem, round(avg(JSONExtractFloat(details, 'net_rx_bytes') / 1048576), 2) AS rx, round(avg(JSONExtractFloat(details, 'net_tx_bytes') / 1048576), 2) AS tx FROM cloudflow.ebpf_events WHERE probe_id = 'stb-188-bpf' AND category = 'metrics' AND timestamp > now() - INTERVAL 24 HOUR GROUP BY toStartOfInterval(timestamp, INTERVAL 1 MINUTE) ORDER BY toStartOfInterval(timestamp, INTERVAL 1 MINUTE)")
  return parseTSV(res).map(r => ({ time: r[0], cpu: parseFloat(r[1] || '0'), mem: parseFloat(r[2] || '0'), netRx: parseFloat(r[3] || '0'), netTx: parseFloat(r[4] || '0') }))
}

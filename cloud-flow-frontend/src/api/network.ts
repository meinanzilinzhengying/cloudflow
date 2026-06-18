import request from '@/utils/request'
import type { ApiResponse } from '@/utils/request'

export interface FlowRecord {
  timestamp: string
  srcIp: string
  dstIp: string
  srcPort: number
  dstPort: number
  protocol: string
  bytes: number
  packets: number
  rtt: number
}

export interface NetworkStats {
  flowTrend: { time: string; value: number }[]
  commMatrix: { src: string; dst: string; value: number }[]
  topology: { nodes: any[]; links: any[] }
  flows: FlowRecord[]
}

export function getNetworkFlows(params: Record<string, any>): Promise<ApiResponse<NetworkStats>> {
  return request.get('/network/flows', { params })
}

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
  // 探针API字段映射 (src/dst/sport/dport/time)
  src?: string
  dst?: string
  sport?: number
  dport?: number
  time?: string
  status?: string
}

export interface NetworkTrend {
  time: string
  rx: number
  tx: number
  pps: number
}

export interface NetworkTopologyNode {
  id: string
  name: string
  type: string
}

export interface NetworkStats {
  flowTrend: NetworkTrend[]
  flows: FlowRecord[]
}

// 探针路由: /api/v1/network/flows
export function getNetworkFlows(params: Record<string, any>): Promise<ApiResponse<FlowRecord[]>> {
  return request.get('/network/flows', { params })
}

// 探针路由: /api/v1/network/trends
export function getNetworkTrends(params?: Record<string, any>): Promise<ApiResponse<NetworkTrend[]>> {
  return request.get('/network/trends', { params })
}

// 探针路由: /api/v1/network/topology
export function getNetworkTopology(): Promise<ApiResponse<any>> {
  return request.get('/network/topology')
}

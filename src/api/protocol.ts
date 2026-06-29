import request from '@/utils/request'
import type { ApiResponse } from '@/utils/request'

export interface HTTPLog {
  timestamp: string
  clientIp: string
  method: string
  url: string
  statusCode: number
  latency: number
  userAgent: string
}

export interface DNSLog {
  timestamp: string
  client: string
  domain: string
  qType: string
  response: string
  latency: number
  isNxdomain: boolean
}

export interface ProtocolStats {
  http: {
    reqRate: number
    okRate: number
    errRate: number
    avgLatency: number
    logs: HTTPLog[]
  }
  dns: {
    queryRate: number
    nxdomainRate: number
    topDomain: string
    avgLatency: number
    logs: DNSLog[]
  }
}

export function getProtocolData(type: 'http' | 'dns', params: Record<string, any>): Promise<ApiResponse<ProtocolStats>> {
  return request.get(`/protocol/${type}`, { params })
}

import request from '@/utils/request'
import type { ApiResponse } from '@/utils/request'

export interface PerformanceData {
  cpuLatency: { p50: number; p95: number; p99: number; histogram: { bin: string; count: number }[] }
  memAlloc: { size: string; count: number }[]
  blockIOLatency: { bin: string; count: number }[]
  topProcesses: { pid: number; comm: string; cpu: number; mem: number }[]
}

export function getPerformance(host: string, params: Record<string, any>): Promise<ApiResponse<PerformanceData>> {
  return request.get(`/performance/${host}`, { params })
}

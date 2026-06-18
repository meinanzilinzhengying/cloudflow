import request from '@/utils/request'
import type { ApiResponse } from '@/utils/request'

export interface DashboardOverview {
  probeOnline: number
  probeTotal: number
  todayTraffic: string
  trafficTrend: string
  activeAlerts: number
  alertTrend: string
  monitoredHosts: number
  hostTrend: string
  flowTrend: { time: string; rx: number; tx: number }[]
  protocolDist: { name: string; value: number }[]
  topHosts: { ip: string; bytes: number; percent: string }[]
  recentAlerts: { time: string; level: string; message: string }[]
}

export function getOverview(): Promise<ApiResponse<DashboardOverview>> {
  return request.get('/dashboard/overview')
}

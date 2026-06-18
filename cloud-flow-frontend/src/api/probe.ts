import request from '@/utils/request'
import type { ApiResponse } from '@/utils/request'

export interface Probe {
  id: string
  hostname: string
  ip: string
  version: string
  status: 'online' | 'degraded' | 'offline'
  cpu: number
  memory: number
  kernel: string
  btf: boolean
  uptime: number
  collectors: string[]
}

export interface ProbeDetail extends Probe {
  arch: string
  platform: string
  hooks: string[]
  ringBufferSize: string
  lastHeartbeat: string
}

export function getProbes(): Promise<ApiResponse<Probe[]>> {
  return request.get('/probes')
}

export function getProbeDetail(id: string): Promise<ApiResponse<ProbeDetail>> {
  return request.get(`/probes/${id}`)
}

export function updateProbeConfig(id: string, config: Record<string, any>): Promise<ApiResponse<void>> {
  return request.put(`/probes/${id}/config`, config)
}

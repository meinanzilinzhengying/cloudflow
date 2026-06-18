import request from '@/utils/request'
import type { ApiResponse } from '@/utils/request'

export interface SecurityEvent {
  id: string
  timestamp: string
  level: 'high' | 'medium' | 'low'
  status: 'pending' | 'handled' | 'ignored'
  type: string
  description: string
  host: string
  rawData: string
  suggestion: string
}

export function getSecurityEvents(params: Record<string, any>): Promise<ApiResponse<SecurityEvent[]>> {
  return request.get('/security/events', { params })
}

export function updateEventStatus(id: string, status: string): Promise<ApiResponse<void>> {
  return request.put(`/security/events/${id}`, { status })
}

import request from '@/utils/request'
import type { ApiResponse } from '@/utils/request'

export interface CpuDataPoint {
  time: string
  usage: number
  user: number
  system: number
  iowait: number
}

export interface MemoryData {
  total: number
  free: number
  buffers: number
  cache: number
  trend: { time: string; used: number; free: number }[]
}

export interface DiskDataPoint {
  time: string
  read: number
  write: number
  ioutil: number
}

export interface ProcessRecord {
  pid: number
  name: string
  cpu: number
  memory: number
  io: number
  threads: number
  status: string
}

export interface PerformanceData {
  topProcesses: ProcessRecord[]
}

// /api/v1/performance/cpu - 返回CPU趋势数组
export function getPerformanceCpu(): Promise<ApiResponse<CpuDataPoint[]>> {
  return request.get('/performance/cpu')
}

// /api/v1/performance/memory - 返回内存对象(total/free/trend)
export function getPerformanceMemory(): Promise<ApiResponse<MemoryData>> {
  return request.get('/performance/memory')
}

// /api/v1/performance/disk - 返回磁盘IO趋势
export function getPerformanceDisk(): Promise<ApiResponse<DiskDataPoint[]>> {
  return request.get('/performance/disk')
}

// /api/v1/performance/process - 返回进程列表
export function getPerformanceProcess(): Promise<ApiResponse<ProcessRecord[]>> {
  return request.get('/performance/process')
}

// 兼容旧接口
export function getPerformance(_host: string, params: Record<string, any>): Promise<ApiResponse<PerformanceData>> {
  return request.get('/performance/process', { params })
}

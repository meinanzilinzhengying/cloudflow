import request from '@/utils/request'
import type { ApiResponse } from '@/utils/request'

export interface LoginParams {
  username: string
  password: string
}

export interface LoginResult {
  token: string
  username: string
}

export function login(data: LoginParams): Promise<ApiResponse<LoginResult>> {
  return request.post('/auth/login', data)
}

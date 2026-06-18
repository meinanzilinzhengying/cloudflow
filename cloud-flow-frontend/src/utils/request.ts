import axios, { type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'

export interface ApiResponse<T> {
  code: number
  message: string
  data: T
}

const instance = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
})

instance.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  const token = localStorage.getItem('cf_token')
  if (token) {
    config.headers.set('Authorization', `Bearer ${token}`)
  }
  return config
})

instance.interceptors.response.use(
  (res: AxiosResponse) => {
    return res.data as unknown as AxiosResponse
  },
  (err) => {
    if (err.response?.status === 401) {
      localStorage.removeItem('cf_token')
      window.location.href = '/#/login'
    }
    return Promise.reject(err)
  }
)

export default instance

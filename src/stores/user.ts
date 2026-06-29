import { ref } from 'vue'
import { defineStore } from 'pinia'

export const useUserStore = defineStore('user', () => {
  const token = ref<string>(localStorage.getItem('cf_token') || '')
  const username = ref<string>(localStorage.getItem('cf_user') || '')

  const setToken = (t: string, u: string) => {
    token.value = t
    username.value = u
    localStorage.setItem('cf_token', t)
    localStorage.setItem('cf_user', u)
  }

  const logout = () => {
    token.value = ''
    username.value = ''
    localStorage.removeItem('cf_token')
    localStorage.removeItem('cf_user')
  }

  return { token, username, setToken, logout }
})

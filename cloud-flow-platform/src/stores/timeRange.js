import { ref, watch } from 'vue'

const STORAGE_KEY = 'cloudflow_time_range'
const saved = typeof window !== 'undefined' ? localStorage.getItem(STORAGE_KEY) : null
export const timeRange = ref(saved || '5m')

watch(timeRange, (val) => {
  if (typeof window !== 'undefined') {
    localStorage.setItem(STORAGE_KEY, val)
  }
})

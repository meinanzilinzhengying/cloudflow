<template>
  <header class="h-14 bg-dark-200 border-b border-dark-100 flex items-center justify-between px-6">
    <div class="flex items-center gap-3">
      <div class="w-8 h-8 rounded-lg bg-accent-500 flex items-center justify-center">
        <Cloud class="w-5 h-5 text-dark-300" />
      </div>
      <h1 class="text-lg font-semibold text-white">云流量平台自监控</h1>
    </div>
    <div class="flex items-center gap-4">
      <span class="text-sm text-gray-400">{{ currentTime }}</span>
      <button class="p-2 hover:bg-dark-100 rounded-lg transition-colors">
        <Bell class="w-5 h-5 text-gray-400" />
      </button>
      <div class="flex items-center gap-2">
        <div class="w-8 h-8 rounded-full bg-accent-500 flex items-center justify-center text-dark-300 font-medium">
          A
        </div>
        <span class="text-sm">Admin</span>
      </div>
    </div>
  </header>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { Cloud, Bell } from 'lucide-vue-next'

defineProps({
  title: {
    type: String,
    default: ''
  }
})

const currentTime = ref('')
let timer = null

function updateTime() {
  const now = new Date()
  currentTime.value = now.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  })
}

onMounted(() => {
  updateTime()
  timer = setInterval(updateTime, 1000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

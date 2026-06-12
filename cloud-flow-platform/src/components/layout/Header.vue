<template>
  <header class="h-16 bg-dark-800 border-b border-dark-600 px-6 flex items-center justify-between sticky top-0 z-30">
    <div class="flex items-center gap-4">
      <div class="relative">
        <Search class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
        <input
          v-model="searchQuery"
          type="text"
          placeholder="搜索..."
          class="w-64 pl-10 pr-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-sm text-white placeholder-gray-500 focus:outline-none focus:border-primary-500 transition"
        />
      </div>
    </div>
    
    <div class="flex items-center gap-4">
      <div class="flex items-center gap-2 bg-dark-700 px-3 py-1.5 rounded-lg">
        <button
          v-for="range in timeRanges"
          :key="range.value"
          @click="selectTimeRange(range.value)"
          :class="[
            'px-3 py-1.5 text-xs font-medium rounded-md transition',
            selectedRange === range.value
              ? 'bg-primary-500 text-white'
              : 'text-gray-400 hover:text-white hover:bg-dark-600'
          ]"
        >
          {{ range.label }}
        </button>
      </div>
      
      <div class="flex items-center gap-2">
        <button 
          @click="handleRefresh"
          class="p-2 hover:bg-dark-700 rounded-lg transition group"
          title="刷新数据"
        >
          <RefreshCw :class="['w-5 h-5 text-gray-400 group-hover:text-white transition', { 'animate-spin': refreshing }]" />
        </button>
        
        <button 
          @click="toggleFullscreen"
          class="p-2 hover:bg-dark-700 rounded-lg transition"
          title="大屏模式"
        >
          <Maximize2 class="w-5 h-5 text-gray-400 hover:text-white" />
        </button>
        
        <button class="relative p-2 hover:bg-dark-700 rounded-lg transition">
          <Bell class="w-5 h-5 text-gray-400 hover:text-white" />
          <span class="absolute top-1 right-1 w-2 h-2 bg-red-500 rounded-full"></span>
        </button>
      </div>
    </div>
  </header>
</template>

<script setup>
import { ref } from 'vue'
import { Search, RefreshCw, Maximize2, Bell } from 'lucide-vue-next'

const emit = defineEmits(['refresh', 'timeRangeChange'])

const searchQuery = ref('')
const selectedRange = ref('5m')
const refreshing = ref(false)

const timeRanges = [
  { label: '5分钟', value: '5m' },
  { label: '15分钟', value: '15m' },
  { label: '1小时', value: '1h' },
  { label: '6小时', value: '6h' },
  { label: '1天', value: '1d' },
  { label: '7天', value: '7d' }
]

const selectTimeRange = (range) => {
  selectedRange.value = range
  emit('timeRangeChange', range)
}

const toggleFullscreen = () => {
  if (document.fullscreenElement) {
    document.exitFullscreen()
  } else {
    document.documentElement.requestFullscreen()
  }
}

const handleRefresh = () => {
  refreshing.value = true
  setTimeout(() => {
    refreshing.value = false
  }, 500)
}
</script>

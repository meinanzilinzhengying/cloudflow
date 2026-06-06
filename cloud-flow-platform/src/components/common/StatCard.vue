<template>
  <div class="bg-dark-200 rounded-xl p-5 border border-dark-100">
    <div class="flex items-center justify-between mb-3">
      <div class="flex items-center gap-2">
        <component :is="iconComponent" class="w-5 h-5 text-accent-500" />
        <span class="text-sm text-gray-400">{{ title }}</span>
      </div>
      <span v-if="trend" :class="['text-xs px-2 py-0.5 rounded', trendClass]">
        {{ trend }}
      </span>
    </div>
    <div class="text-2xl font-bold text-white mb-1">{{ value }}</div>
    <div class="text-xs text-gray-500">{{ subtitle }}</div>
    <div v-if="percent !== undefined" class="mt-3">
      <div class="flex justify-between text-xs text-gray-500 mb-1">
        <span>使用率</span>
        <span>{{ percent }}%</span>
      </div>
      <div class="h-1.5 bg-dark-300 rounded-full overflow-hidden">
        <div 
          class="h-full bg-accent-500 rounded-full transition-all duration-500"
          :style="{ width: `${percent}%` }"
        ></div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { TrendingUp, TrendingDown } from 'lucide-vue-next'

const props = defineProps({
  title: { type: String, required: true },
  value: { type: [String, Number], required: true },
  subtitle: { type: String, default: '' },
  icon: { type: String, default: 'Activity' },
  trend: { type: String, default: '' },
  percent: { type: Number, default: undefined }
})

const iconComponent = computed(() => {
  const icons = {
    Activity, Cpu, HardDrive, Network, Server, Clock
  }
  return icons[props.icon] || icons.Activity
})

const trendClass = computed(() => {
  if (props.trend?.startsWith('+')) return 'bg-green-500/20 text-green-400'
  if (props.trend?.startsWith('-')) return 'bg-red-500/20 text-red-400'
  return 'bg-gray-500/20 text-gray-400'
})
</script>

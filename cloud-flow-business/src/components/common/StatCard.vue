<template>
  <div 
    class="bg-dark-800 rounded-xl p-5 border border-dark-600 hover:border-primary-500/30 transition-all duration-200"
    :class="{ 'animate-pulse': loading }"
  >
    <div class="flex items-center justify-between">
      <div>
        <p class="text-gray-400 text-sm mb-1">{{ title }}</p>
        <p class="text-2xl font-bold" :class="colorClass">{{ displayValue }}</p>
        <p v-if="change !== undefined" class="text-xs mt-2 flex items-center gap-1" :class="changeClass">
          <component :is="changeIcon" class="w-3 h-3" />
          {{ change }}
        </p>
      </div>
      <div 
        class="w-12 h-12 rounded-lg flex items-center justify-center"
        :class="bgClass"
      >
        <component :is="icon" class="w-6 h-6" :class="iconClass" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { TrendingUp, TrendingDown } from 'lucide-vue-next'

const props = defineProps({
  title: { type: String, required: true },
  value: { type: [Number, String], default: 0 },
  unit: { type: String, default: '' },
  change: { type: [Number, String], default: undefined },
  icon: { type: Object, required: true },
  variant: { type: String, default: 'default' },
  loading: { type: Boolean, default: false }
})

const colorMap = {
  default: 'text-white',
  success: 'text-green-400',
  warning: 'text-yellow-400',
  danger: 'text-red-400',
  info: 'text-blue-400'
}

const bgMap = {
  default: 'bg-dark-700',
  success: 'bg-green-500/20',
  warning: 'bg-yellow-500/20',
  danger: 'bg-red-500/20',
  info: 'bg-blue-500/20'
}

const iconColorMap = {
  default: 'text-gray-400',
  success: 'text-green-400',
  warning: 'text-yellow-400',
  danger: 'text-red-400',
  info: 'text-blue-400'
}

const colorClass = computed(() => colorMap[props.variant] || colorMap.default)
const bgClass = computed(() => bgMap[props.variant] || bgMap.default)
const iconClass = computed(() => iconColorMap[props.variant] || iconColorMap.default)

const displayValue = computed(() => {
  if (props.loading) return '--'
  if (typeof props.value === 'number') {
    return formatNumber(props.value) + (props.unit ? ` ${props.unit}` : '')
  }
  return props.value + (props.unit ? ` ${props.unit}` : '')
})

const changeIcon = computed(() => {
  if (!props.change) return null
  const num = parseFloat(props.change)
  return num >= 0 ? TrendingUp : TrendingDown
})

const changeClass = computed(() => {
  if (!props.change) return ''
  const num = parseFloat(props.change)
  return num >= 0 ? 'text-green-400' : 'text-red-400'
})

const formatNumber = (num) => {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return num.toLocaleString()
}
</script>

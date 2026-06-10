<template>
  <div class="card p-4 hover:shadow-card-hover transition-all duration-300">
    <div class="flex items-start justify-between">
      <div class="flex items-center gap-3">
        <div
          :class="[
            'w-10 h-10 rounded-xl flex items-center justify-center',
            iconBgClass
          ]"
        >
          <component :is="icon" :class="['w-5 h-5', iconColorClass]" />
        </div>
        <div>
          <p class="text-sm font-medium text-slate-500 dark:text-slate-400">{{ title }}</p>
          <div class="flex items-baseline gap-1 mt-1">
            <span v-if="!loading" class="text-2xl font-bold text-slate-900 dark:text-white">
              {{ value }}
            </span>
            <span v-else class="w-12 h-6 bg-slate-200 dark:bg-dark-600 rounded animate-pulse"></span>
            <span class="text-sm text-slate-400">{{ unit }}</span>
          </div>
        </div>
      </div>
      <div
        v-if="change && !loading"
        :class="[
          'flex items-center gap-1 text-xs font-medium px-2 py-1 rounded-lg',
          isPositive ? 'bg-green-50 text-green-600 dark:bg-green-500/10 dark:text-green-400' : 'bg-red-50 text-red-600 dark:bg-red-500/10 dark:text-red-400'
        ]"
      >
        <component :is="isPositive ? TrendingUp : TrendingDown" class="w-3 h-3" />
        {{ change }}
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { TrendingUp, TrendingDown } from 'lucide-vue-next'

const props = defineProps({
  title: { type: String, required: true },
  value: { type: [String, Number], default: '0' },
  unit: { type: String, default: '' },
  change: { type: String, default: '' },
  icon: { type: Object, required: true },
  color: { type: String, default: 'primary' },
  loading: { type: Boolean, default: false },
})

const isPositive = computed(() => {
  if (!props.change) return true
  return props.change.startsWith('+') || (!props.change.startsWith('-') && !isNaN(parseFloat(props.change)))
})

const iconBgClass = computed(() => {
  const classes = {
    primary: 'bg-primary-50 dark:bg-primary-500/10',
    accent: 'bg-accent-50 dark:bg-accent-500/10',
    success: 'bg-green-50 dark:bg-green-500/10',
    warning: 'bg-amber-50 dark:bg-amber-500/10',
    danger: 'bg-red-50 dark:bg-red-500/10',
    violet: 'bg-violet-50 dark:bg-violet-500/10',
  }
  return classes[props.color] || classes.primary
})

const iconColorClass = computed(() => {
  const classes = {
    primary: 'text-primary-500',
    accent: 'text-accent-500',
    success: 'text-green-500',
    warning: 'text-amber-500',
    danger: 'text-red-500',
    violet: 'text-violet-500',
  }
  return classes[props.color] || classes.primary
})
</script>

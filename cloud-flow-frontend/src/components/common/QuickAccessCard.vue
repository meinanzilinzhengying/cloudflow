<template>
  <div
    class="group relative flex flex-col items-center justify-center p-6 rounded-2xl bg-slate-50 dark:bg-dark-700/50 hover:bg-white dark:hover:bg-dark-700 border border-transparent hover:border-slate-200 dark:hover:border-dark-600 cursor-pointer transition-all duration-300 hover:shadow-lg hover:-translate-y-1"
  >
    <!-- Glow Effect -->
    <div
      :class="[
        'absolute inset-0 rounded-2xl opacity-0 group-hover:opacity-100 transition-opacity duration-300',
        glowClass
      ]"
    ></div>

    <div
      :class="[
        'w-14 h-14 rounded-2xl flex items-center justify-center mb-4 transition-all duration-300 group-hover:scale-110',
        iconBgClass
      ]"
    >
      <component :is="icon" :class="['w-7 h-7', iconColorClass]" />
    </div>

    <h4 class="text-base font-semibold text-slate-900 dark:text-white text-center mb-1">
      {{ title }}
    </h4>
    <p class="text-xs text-slate-500 dark:text-slate-400 text-center">
      {{ description }}
    </p>

    <!-- Arrow -->
    <div class="absolute top-4 right-4 opacity-0 group-hover:opacity-100 transition-opacity duration-300">
      <ArrowRight class="w-4 h-4 text-slate-400" />
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { ArrowRight } from 'lucide-vue-next'

const props = defineProps({
  icon: { type: Object, required: true },
  title: { type: String, required: true },
  description: { type: String, default: '' },
  color: { type: String, default: 'primary' },
})

const iconBgClass = computed(() => {
  const classes = {
    primary: 'bg-primary-50 dark:bg-primary-500/10',
    accent: 'bg-accent-50 dark:bg-accent-500/10',
    emerald: 'bg-emerald-50 dark:bg-emerald-500/10',
    amber: 'bg-amber-50 dark:bg-amber-500/10',
    violet: 'bg-violet-50 dark:bg-violet-500/10',
    rose: 'bg-rose-50 dark:bg-rose-500/10',
  }
  return classes[props.color] || classes.primary
})

const iconColorClass = computed(() => {
  const classes = {
    primary: 'text-primary-500',
    accent: 'text-accent-500',
    emerald: 'text-emerald-500',
    amber: 'text-amber-500',
    violet: 'text-violet-500',
    rose: 'text-rose-500',
  }
  return classes[props.color] || classes.primary
})

const glowClass = computed(() => {
  const classes = {
    primary: 'bg-gradient-to-br from-primary-500/5 to-transparent',
    accent: 'bg-gradient-to-br from-accent-500/5 to-transparent',
    emerald: 'bg-gradient-to-br from-emerald-500/5 to-transparent',
    amber: 'bg-gradient-to-br from-amber-500/5 to-transparent',
    violet: 'bg-gradient-to-br from-violet-500/5 to-transparent',
    rose: 'bg-gradient-to-br from-rose-500/5 to-transparent',
  }
  return classes[props.color] || classes.primary
})
</script>

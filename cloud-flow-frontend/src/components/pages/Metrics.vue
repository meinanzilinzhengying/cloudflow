<template>
  <div class="flex items-center justify-center h-[60vh]">
    <div class="text-center">
      <div class="w-20 h-20 mx-auto mb-6 rounded-2xl bg-amber-50 dark:bg-amber-500/10 flex items-center justify-center">
        <BarChart3 class="w-10 h-10 text-amber-500" />
      </div>
      <h2 class="text-2xl font-bold text-slate-900 dark:text-white mb-2">指标监控</h2>
      <p class="text-slate-500 dark:text-slate-400 max-w-md mx-auto">
        支持 Prometheus 指标与 OTEL Metrics，可视化展示系统关键指标
      </p>
    </div>
  </div>
</template>

<script setup>
import { BarChart3 } from 'lucide-vue-next'
</script>

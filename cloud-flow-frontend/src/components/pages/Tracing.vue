<template>
  <div class="flex items-center justify-center h-[60vh]">
    <div class="text-center">
      <div class="w-20 h-20 mx-auto mb-6 rounded-2xl bg-emerald-50 dark:bg-emerald-500/10 flex items-center justify-center">
        <Activity class="w-10 h-10 text-emerald-500" />
      </div>
      <h2 class="text-2xl font-bold text-slate-900 dark:text-white mb-2">链路追踪</h2>
      <p class="text-slate-500 dark:text-slate-400 max-w-md mx-auto">
        基于 OpenTelemetry 的分布式链路追踪，支持 Trace、Span 分析与根因定位
      </p>
    </div>
  </div>
</template>

<script setup>
import { Activity } from 'lucide-vue-next'
</script>

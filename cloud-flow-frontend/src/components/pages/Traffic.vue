<template>
  <div class="flex items-center justify-center h-[60vh]">
    <div class="text-center">
      <div class="w-20 h-20 mx-auto mb-6 rounded-2xl bg-primary-50 dark:bg-primary-500/10 flex items-center justify-center">
        <TrendingUp class="w-10 h-10 text-primary-500" />
      </div>
      <h2 class="text-2xl font-bold text-slate-900 dark:text-white mb-2">流量分析</h2>
      <p class="text-slate-500 dark:text-slate-400 max-w-md mx-auto">
        基于 eBPF 的网络流量实时分析，支持入站/出站流量、PPS、BPS 等多维度监控
      </p>
      <div class="mt-8 flex justify-center gap-4">
        <div class="px-4 py-2 rounded-xl bg-slate-100 dark:bg-dark-700">
          <p class="text-2xl font-bold text-primary-500">0</p>
          <p class="text-xs text-slate-500">Gbps 总流量</p>
        </div>
        <div class="px-4 py-2 rounded-xl bg-slate-100 dark:bg-dark-700">
          <p class="text-2xl font-bold text-accent-500">0</p>
          <p class="text-xs text-slate-500">K PPS</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { TrendingUp } from 'lucide-vue-next'
</script>

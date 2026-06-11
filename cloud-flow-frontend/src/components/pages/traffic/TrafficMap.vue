<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">流量地图</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">可视化网络流量分布</p>
      </div>
    </div>

    <div class="card p-6">
      <div class="h-96 bg-gradient-to-br from-primary-50 dark:from-primary-500/10 to-accent-50 dark:to-accent-500/10 rounded-xl flex items-center justify-center">
        <div class="text-center">
          <Globe class="w-16 h-16 mx-auto mb-4 text-primary-400" />
          <p class="text-slate-500 dark:text-slate-400">流量地图功能开发中</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { Globe } from 'lucide-vue-next'
</script>

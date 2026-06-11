<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">拓扑变更对比</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">对比不同时间点的拓扑变化</p>
      </div>
      <div class="flex items-center gap-3">
        <input type="text" placeholder="时间点1" class="input max-w-xs" />
        <ArrowRight class="w-5 h-5 text-slate-400" />
        <input type="text" placeholder="时间点2" class="input max-w-xs" />
        <button class="btn-primary">
          <GitCompare class="w-4 h-4" />
          对比
        </button>
      </div>
    </div>

    <div class="card p-6">
      <div class="h-[500px] flex items-center justify-center">
        <div class="text-center">
          <GitCompare class="w-16 h-16 mx-auto mb-4 text-primary-400" />
          <p class="text-slate-500 dark:text-slate-400">拓扑对比功能开发中</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ArrowRight, GitCompare } from 'lucide-vue-next'
</script>

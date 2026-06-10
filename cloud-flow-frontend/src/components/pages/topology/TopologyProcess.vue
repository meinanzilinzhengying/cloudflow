<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">进程拓扑</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">进程级别的依赖关系</p>
      </div>
    </div>

    <div class="card p-6">
      <div class="h-[500px] flex items-center justify-center">
        <div class="text-center">
          <Cpu class="w-16 h-16 mx-auto mb-4 text-primary-400" />
          <p class="text-slate-500 dark:text-slate-400">进程拓扑功能开发中</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { Cpu } from 'lucide-vue-next'
</script>

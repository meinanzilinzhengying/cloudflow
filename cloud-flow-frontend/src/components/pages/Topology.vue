<template>
  <div class="flex items-center justify-center h-[60vh]">
    <div class="text-center">
      <div class="w-20 h-20 mx-auto mb-6 rounded-2xl bg-accent-50 dark:bg-accent-500/10 flex items-center justify-center">
        <Layers class="w-10 h-10 text-accent-500" />
      </div>
      <h2 class="text-2xl font-bold text-slate-900 dark:text-white mb-2">服务拓扑可视化</h2>
      <p class="text-slate-500 dark:text-slate-400 max-w-md mx-auto">
        自动发现服务依赖关系，可视化展示服务调用链路与健康状态
      </p>
    </div>
  </div>
</template>

<script setup>
import { Layers } from 'lucide-vue-next'
</script>

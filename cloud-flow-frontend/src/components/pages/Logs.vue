<template>
  <div class="flex items-center justify-center h-[60vh]">
    <div class="text-center">
      <div class="w-20 h-20 mx-auto mb-6 rounded-2xl bg-violet-50 dark:bg-violet-500/10 flex items-center justify-center">
        <FileText class="w-10 h-10 text-violet-500" />
      </div>
      <h2 class="text-2xl font-bold text-slate-900 dark:text-white mb-2">日志分析</h2>
      <p class="text-slate-500 dark:text-slate-400 max-w-md mx-auto">
        统一收集与分析应用日志，支持日志搜索、过滤与实时流
      </p>
    </div>
  </div>
</template>

<script setup>
import { FileText } from 'lucide-vue-next'
</script>

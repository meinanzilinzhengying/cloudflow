<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">流量回放</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">历史流量数据回放分析</p>
      </div>
      <button class="btn-primary">
        <Play class="w-4 h-4" />
        开始回放
      </button>
    </div>

    <div class="card p-6">
      <div class="flex items-center gap-4 mb-6">
        <input type="text" placeholder="选择时间点..." class="input max-w-xs" />
        <div class="flex-1">
          <div class="h-2 bg-slate-200 dark:bg-dark-700 rounded-full relative">
            <div class="absolute left-1/2 top-0 w-4 h-4 bg-primary-500 rounded-full -translate-x-1/2 -translate-y-1"></div>
          </div>
        </div>
        <button class="btn-secondary">
          <SkipBack class="w-4 h-4" />
        </button>
        <button class="btn-primary">
          <Play class="w-4 h-4" />
        </button>
        <button class="btn-secondary">
          <SkipForward class="w-4 h-4" />
        </button>
        <span class="text-sm text-slate-500">00:00 / 05:32</span>
      </div>
      <div class="h-64 bg-slate-50 dark:bg-dark-700 rounded-xl flex items-center justify-center">
        <div class="text-center">
          <History class="w-12 h-12 mx-auto mb-3 text-primary-400" />
          <p class="text-slate-500 dark:text-slate-400">选择时间点开始回放</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { Play, SkipBack, SkipForward, History } from 'lucide-vue-next'
</script>

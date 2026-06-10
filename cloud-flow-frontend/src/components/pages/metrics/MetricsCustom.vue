<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">自定义指标</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">管理和查看自定义指标</p>
      </div>
      <button class="btn-primary">
        <Plus class="w-4 h-4" />
        添加指标
      </button>
    </div>

    <div class="card p-6">
      <div class="h-64 flex items-center justify-center">
        <div class="text-center">
          <Activity class="w-16 h-16 mx-auto mb-4 text-primary-400" />
          <p class="text-slate-500 dark:text-slate-400">自定义指标功能开发中</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { Plus, Activity } from 'lucide-vue-next'
</script>

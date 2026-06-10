<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">租户管理</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">管理平台租户与项目资源</p>
      </div>
      <button class="btn-primary">
        <Plus class="w-4 h-4" />
        创建租户
      </button>
    </div>

    <!-- Tenant List -->
    <div class="card">
      <div class="p-8 text-center text-slate-500">
        <Building2 class="w-12 h-12 mx-auto mb-4 opacity-50" />
        <p>暂无租户数据</p>
      </div>
    </div>
  </div>
</template>

<script setup>
import { Plus, Building2 } from 'lucide-vue-next'
</script>

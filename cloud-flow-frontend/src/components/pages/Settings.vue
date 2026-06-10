<template>
  <div class="space-y-6">
    <div>
      <h1 class="text-2xl font-bold text-slate-900 dark:text-white">系统设置</h1>
      <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">配置系统参数与集成</p>
    </div>

    <div class="grid grid-cols-3 gap-6">
      <div class="card p-6">
        <div class="w-12 h-12 rounded-xl bg-primary-50 dark:bg-primary-500/10 flex items-center justify-center mb-4">
          <Settings class="w-6 h-6 text-primary-500" />
        </div>
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-2">通用设置</h3>
        <p class="text-sm text-slate-500">语言、时区、主题等基本配置</p>
      </div>
      <div class="card p-6">
        <div class="w-12 h-12 rounded-xl bg-accent-50 dark:bg-accent-500/10 flex items-center justify-center mb-4">
          <Shield class="w-6 h-6 text-accent-500" />
        </div>
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-2">安全设置</h3>
        <p class="text-sm text-slate-500">认证、授权、API Key 管理</p>
      </div>
      <div class="card p-6">
        <div class="w-12 h-12 rounded-xl bg-emerald-50 dark:bg-emerald-500/10 flex items-center justify-center mb-4">
          <Plug class="w-6 h-6 text-emerald-500" />
        </div>
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-2">集成设置</h3>
        <p class="text-sm text-slate-500">外部系统、通知渠道配置</p>
      </div>
    </div>
  </div>
</template>

<script setup>
import { Settings, Shield, Plug } from 'lucide-vue-next'
</script>

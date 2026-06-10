<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">告警中心</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">管理和监控所有告警事件</p>
      </div>
      <button class="btn-primary">
        <Plus class="w-4 h-4" />
        创建告警规则
      </button>
    </div>

    <!-- Stats -->
    <div class="grid grid-cols-4 gap-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">全部告警</p>
        <p class="text-2xl font-bold text-slate-900 dark:text-white">0</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">触发中</p>
        <p class="text-2xl font-bold text-red-500">0</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">已解决</p>
        <p class="text-2xl font-bold text-green-500">0</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">静默中</p>
        <p class="text-2xl font-bold text-amber-500">0</p>
      </div>
    </div>

    <!-- Alert List -->
    <div class="card">
      <div class="p-4 border-b border-slate-200 dark:border-dark-700">
        <div class="flex items-center gap-4">
          <input type="text" placeholder="搜索告警..." class="input max-w-xs" />
          <select class="input w-32">
            <option>全部等级</option>
            <option>严重</option>
            <option>警告</option>
            <option>信息</option>
          </select>
          <select class="input w-32">
            <option>全部状态</option>
            <option>触发中</option>
            <option>已解决</option>
          </select>
        </div>
      </div>
      <div class="p-8 text-center text-slate-500">
        <Bell class="w-12 h-12 mx-auto mb-4 opacity-50" />
        <p>暂无告警数据</p>
      </div>
    </div>
  </div>
</template>

<script setup>
import { Plus, Bell } from 'lucide-vue-next'
</script>

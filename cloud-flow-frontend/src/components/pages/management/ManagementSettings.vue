<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">系统设置</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">配置系统参数</p>
      </div>
      <button class="btn-primary">
        <Save class="w-4 h-4" />
        保存配置
      </button>
    </div>

    <div class="grid grid-cols-2 gap-6">
      <!-- General Settings -->
      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-6">通用设置</h3>
        <div class="space-y-6">
          <div>
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-200 mb-2">系统名称</label>
            <input type="text" value="CloudFlow" class="input w-full" />
          </div>
          <div>
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-200 mb-2">系统描述</label>
            <textarea class="input w-full" rows="3">企业级云原生可观测平台</textarea>
          </div>
          <div class="flex items-center justify-between">
            <span class="text-sm font-medium text-slate-700 dark:text-slate-200">深色模式</span>
            <button class="relative w-12 h-6 rounded-full transition-colors" :class="darkMode ? 'bg-primary-500' : 'bg-slate-300'">
              <span class="absolute top-1 w-4 h-4 rounded-full bg-white shadow transition-transform" :class="darkMode ? 'translate-x-7' : 'translate-x-1'"></span>
            </button>
          </div>
        </div>
      </div>

      <!-- Notification Settings -->
      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-6">通知设置</h3>
        <div class="space-y-6">
          <div class="flex items-center justify-between">
            <span class="text-sm font-medium text-slate-700 dark:text-slate-200">邮件通知</span>
            <button class="relative w-12 h-6 rounded-full transition-colors" :class="emailEnabled ? 'bg-primary-500' : 'bg-slate-300'">
              <span class="absolute top-1 w-4 h-4 rounded-full bg-white shadow transition-transform" :class="emailEnabled ? 'translate-x-7' : 'translate-x-1'"></span>
            </button>
          </div>
          <div class="flex items-center justify-between">
            <span class="text-sm font-medium text-slate-700 dark:text-slate-200">钉钉通知</span>
            <button class="relative w-12 h-6 rounded-full transition-colors" :class="dingtalkEnabled ? 'bg-primary-500' : 'bg-slate-300'">
              <span class="absolute top-1 w-4 h-4 rounded-full bg-white shadow transition-transform" :class="dingtalkEnabled ? 'translate-x-7' : 'translate-x-1'"></span>
            </button>
          </div>
          <div class="flex items-center justify-between">
            <span class="text-sm font-medium text-slate-700 dark:text-slate-200">电话通知</span>
            <button class="relative w-12 h-6 rounded-full transition-colors" :class="phoneEnabled ? 'bg-primary-500' : 'bg-slate-300'">
              <span class="absolute top-1 w-4 h-4 rounded-full bg-white shadow transition-transform" :class="phoneEnabled ? 'translate-x-7' : 'translate-x-1'"></span>
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { Save } from 'lucide-vue-next'

const darkMode = ref(false)
const emailEnabled = ref(true)
const dingtalkEnabled = ref(true)
const phoneEnabled = ref(false)
</script>

<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">系统设置</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">配置系统参数</p>
      </div>
    </div>

    <div class="grid grid-cols-2 gap-6">
      <div class="card p-6">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-semibold text-slate-900 dark:text-white">版本信息</h3>
          <button @click="fetchData" class="text-sm text-primary-500 hover:text-primary-600">刷新</button>
        </div>

        <div v-if="loading" class="flex items-center justify-center py-6">
          <Loader2 class="w-6 h-6 text-primary-500 animate-spin" />
        </div>

        <div v-else class="space-y-3">
          <div class="flex items-center justify-between py-2 border-b border-slate-100 dark:border-dark-600">
            <span class="text-sm text-slate-500">系统名称</span>
            <span class="text-sm font-medium text-slate-900 dark:text-white">{{ settings.name || 'CloudFlow' }}</span>
          </div>
          <div class="flex items-center justify-between py-2 border-b border-slate-100 dark:border-dark-600">
            <span class="text-sm text-slate-500">版本</span>
            <span class="text-sm font-medium text-slate-900 dark:text-white">{{ settings.version || settings.api_version || '-' }}</span>
          </div>
          <div class="flex items-center justify-between py-2 border-b border-slate-100 dark:border-dark-600">
            <span class="text-sm text-slate-500">API URL</span>
            <span class="text-sm font-medium text-slate-900 dark:text-white">{{ settings.api_url || settings.apiUrl || '-' }}</span>
          </div>
          <div class="flex items-center justify-between py-2 border-b border-slate-100 dark:border-dark-600">
            <span class="text-sm text-slate-500">构建时间</span>
            <span class="text-sm font-medium text-slate-900 dark:text-white">{{ settings.build_time || settings.buildTime || '-' }}</span>
          </div>
          <div class="flex items-center justify-between py-2">
            <span class="text-sm text-slate-500">环境</span>
            <span class="text-sm font-medium text-slate-900 dark:text-white">{{ settings.env || settings.environment || '-' }}</span>
          </div>
        </div>
      </div>

      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">主题设置</h3>
        <div class="space-y-3">
          <div class="flex items-center justify-between py-2 border-b border-slate-100 dark:border-dark-600">
            <span class="text-sm text-slate-500">深色模式</span>
            <button class="relative w-12 h-6 rounded-full transition-colors bg-primary-500">
              <span class="absolute top-1 w-4 h-4 rounded-full bg-white shadow transition-transform translate-x-7"></span>
            </button>
          </div>
          <div class="flex items-center justify-between py-2 border-b border-slate-100 dark:border-dark-600">
            <span class="text-sm text-slate-500">邮件通知</span>
            <button class="relative w-12 h-6 rounded-full transition-colors bg-slate-300">
              <span class="absolute top-1 w-4 h-4 rounded-full bg-white shadow transition-transform translate-x-1"></span>
            </button>
          </div>
          <div class="flex items-center justify-between py-2">
            <span class="text-sm text-slate-500">钉钉通知</span>
            <button class="relative w-12 h-6 rounded-full transition-colors bg-slate-300">
              <span class="absolute top-1 w-4 h-4 rounded-full bg-white shadow transition-transform translate-x-1"></span>
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { Loader2 } from 'lucide-vue-next'
import { queryService, controlPlaneService } from '../../../api'

const loading = ref(false)
const settings = ref({})

const fetchData = async () => {
  loading.value = true
  try {
    let data = null
    try {
      data = await queryService.getOverview()
    } catch (e) {
      // ignore
    }
    if (!data) {
      try {
        data = await controlPlaneService.getEdges()
      } catch (e) {
        // ignore
      }
    }
    if (data && typeof data === 'object' && !Array.isArray(data)) {
      settings.value = data.data || data.settings || data.system || data.info || data
    } else {
      settings.value = {}
    }
  } catch (err) {
    settings.value = {}
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>

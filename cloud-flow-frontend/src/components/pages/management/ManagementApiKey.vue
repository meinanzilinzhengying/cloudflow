<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">API Key</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">管理API访问密钥</p>
      </div>
      <button class="btn-primary">
        <Plus class="w-4 h-4" />
        创建Key
      </button>
    </div>
    <div class="card">
      <div v-if="loading" class="p-8 text-center text-slate-500">加载中...</div>
      <div v-else-if="apiKeys.length === 0" class="p-8 text-center text-slate-500">暂无API Key</div>
      <div v-else class="overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="bg-slate-50 dark:bg-dark-700/50">
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">名称</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">Key</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">创建时间</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">状态</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr v-for="key in apiKeys" :key="key.id">
              <td class="px-6 py-4 text-sm font-medium text-slate-900 dark:text-white">{{ key.name }}</td>
              <td class="px-6 py-4">
                <code class="text-sm text-primary-500 font-mono">{{ key.key }}</code>
              </td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ key.createdAt }}</td>
              <td class="px-6 py-4">
                <span :class="['text-xs px-2 py-1 rounded-full font-medium', key.enabled ? 'bg-green-100 text-green-600' : 'bg-gray-100 text-gray-600']">
                  {{ key.enabled ? '启用' : '禁用' }}
                </span>
              </td>
              <td class="px-6 py-4">
                <button class="text-xs text-red-500 hover:text-red-600">删除</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { Plus } from 'lucide-vue-next'
import { authService } from '@/api'

const apiKeys = ref([])
const loading = ref(false)

const fetchApiKeys = async () => {
  loading.value = true
  try {
    const res = await authService.getApiKeys()
    apiKeys.value = res.data || []
  } catch (e) {
    console.error('Failed to fetch API keys:', e)
    apiKeys.value = []
  } finally {
    loading.value = false
  }
}

onMounted(fetchApiKeys)
</script>

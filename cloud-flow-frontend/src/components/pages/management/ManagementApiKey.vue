<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">API Key</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">管理API访问密钥</p>
      </div>
    </div>

    <div class="card">
      <div class="overflow-x-auto">
        <div v-if="loading" class="flex items-center justify-center py-12">
          <Loader2 class="w-8 h-8 text-primary-500 animate-spin" />
        </div>

        <div v-else-if="apiKeys.length === 0" class="flex flex-col items-center justify-center py-12 text-slate-500">
          <Inbox class="w-12 h-12 mb-3 text-slate-300" />
          <p>暂无数据</p>
        </div>

        <table v-else class="w-full">
          <thead>
            <tr class="bg-slate-50 dark:bg-dark-700/50">
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">名称</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">Key</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">创建时间</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">状态</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr v-for="(key, idx) in apiKeys" :key="idx">
              <td class="px-6 py-3 text-sm font-medium text-slate-900 dark:text-white">
                {{ key.name || key.key_name || key.title || '-' }}
              </td>
              <td class="px-6 py-3">
                <code class="text-sm text-primary-500 font-mono">
                  {{ key.key || key.api_key || key.apiKey || key.token || '-' }}
                </code>
              </td>
              <td class="px-6 py-3 text-sm text-slate-500">
                {{ key.created_at || key.createTime || key.createdAt || '-' }}
              </td>
              <td class="px-6 py-3">
                <span :class="['text-xs px-2 py-1 rounded-full font-medium', (key.enabled || key.active || key.status === 'active') ? 'bg-green-100 text-green-600' : 'bg-gray-100 text-gray-600']">
                  {{ (key.enabled || key.active || key.status === 'active') ? '启用' : '禁用' }}
                </span>
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
import { Loader2, Inbox } from 'lucide-vue-next'
import { authService, alertService } from '../../../api'

const loading = ref(false)
const apiKeys = ref([])

const fetchData = async () => {
  loading.value = true
  try {
    let data = null
    try {
      if (authService.getApiKeys) {
        data = await authService.getApiKeys()
      }
    } catch (e) {
      // ignore
    }
    if (!data) {
      try {
        if (alertService.getApiKeys) {
          data = await alertService.getApiKeys()
        }
      } catch (e) {
        // ignore
      }
    }
    if (data) {
      const list = Array.isArray(data) ? data : (data.data || data.items || data.results || data.api_keys || data.keys || [])
      apiKeys.value = list
    } else {
      apiKeys.value = []
    }
  } catch (err) {
    apiKeys.value = []
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>

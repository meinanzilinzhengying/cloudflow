<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">Agent管理</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">管理和监控Agent节点</p>
      </div>
      <div class="flex items-center gap-3">
        <button class="btn-secondary" @click="fetchData">
          <RefreshCw class="w-4 h-4" />
          刷新
        </button>
      </div>
    </div>

    <div class="grid grid-cols-4 gap-4">
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">Agent总数</p>
            <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">{{ agentStats.total }}</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-primary-50 dark:bg-primary-500/10 flex items-center justify-center">
            <Server class="w-5 h-5 text-primary-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">在线数</p>
            <p class="text-2xl font-bold text-emerald-500 mt-1">{{ agentStats.online }}</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-emerald-50 dark:bg-emerald-500/10 flex items-center justify-center">
            <CheckCircle class="w-5 h-5 text-emerald-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">离线数</p>
            <p class="text-2xl font-bold text-amber-500 mt-1">{{ agentStats.offline }}</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-amber-50 dark:bg-amber-500/10 flex items-center justify-center">
            <AlertCircle class="w-5 h-5 text-amber-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">异常数</p>
            <p class="text-2xl font-bold text-red-500 mt-1">{{ agentStats.error }}</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-red-50 dark:bg-red-500/10 flex items-center justify-center">
            <AlertTriangle class="w-5 h-5 text-red-500" />
          </div>
        </div>
      </div>
    </div>

    <div class="card">
      <div class="overflow-x-auto">
        <div v-if="loading" class="flex items-center justify-center py-12">
          <Loader2 class="w-8 h-8 text-primary-500 animate-spin" />
        </div>

        <div v-else-if="agents.length === 0" class="flex flex-col items-center justify-center py-12 text-slate-500">
          <Inbox class="w-12 h-12 mb-3 text-slate-300" />
          <p>暂无数据</p>
        </div>

        <table v-else class="w-full">
          <thead>
            <tr class="bg-slate-50 dark:bg-dark-700/50">
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">Agent ID</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">主机名</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">IP地址</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">版本</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">状态</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">最后心跳</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr v-for="(agent, idx) in agents" :key="idx">
              <td class="px-6 py-3">
                <span class="text-sm font-medium text-slate-900 dark:text-white">{{ agent.id || agent.agent_id || agent.agentId || '-' }}</span>
              </td>
              <td class="px-6 py-3 text-sm text-slate-700 dark:text-slate-200">{{ agent.hostname || agent.host_name || agent.name || '-' }}</td>
              <td class="px-6 py-3 text-sm text-slate-500">{{ agent.ip || agent.ip_address || agent.ipAddress || '-' }}</td>
              <td class="px-6 py-3 text-sm text-slate-500">{{ agent.version || agent.agent_version || '-' }}</td>
              <td class="px-6 py-3">
                <span :class="['text-xs px-2 py-1 rounded-full font-medium', statusClass(agent.status)]">
                  {{ statusText(agent.status) }}
                </span>
              </td>
              <td class="px-6 py-3 text-sm text-slate-500">{{ agent.last_heartbeat || agent.lastHeartbeat || agent.heartbeat || agent.updated_at || '-' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Server, CheckCircle, AlertCircle, AlertTriangle, RefreshCw, Loader2, Inbox } from 'lucide-vue-next'
import { controlPlaneService } from '../../../api'

const loading = ref(false)
const agents = ref([])

const normalizeStatus = (status) => {
  if (!status) return 'unknown'
  const s = String(status).toLowerCase()
  if (s === 'online' || s === 'running' || s === 'active' || s === 'connected' || s === 'up') return 'online'
  if (s === 'offline' || s === 'disconnected' || s === 'down' || s === 'inactive') return 'offline'
  if (s === 'error' || s === 'failed' || s === 'critical' || s === 'abnormal') return 'error'
  return s
}

const statusClass = (status) => {
  const s = normalizeStatus(status)
  if (s === 'online') return 'bg-green-100 text-green-600'
  if (s === 'offline') return 'bg-gray-100 text-gray-600'
  if (s === 'error') return 'bg-red-100 text-red-600'
  return 'bg-slate-100 text-slate-600'
}

const statusText = (status) => {
  const s = normalizeStatus(status)
  if (s === 'online') return '在线'
  if (s === 'offline') return '离线'
  if (s === 'error') return '异常'
  return status || '未知'
}

const agentStats = computed(() => {
  const total = agents.value.length
  let online = 0, offline = 0, error = 0
  agents.value.forEach((a) => {
    const s = normalizeStatus(a.status)
    if (s === 'online') online++
    else if (s === 'offline') offline++
    else if (s === 'error') error++
  })
  return { total, online, offline, error }
})

const fetchData = async () => {
  loading.value = true
  try {
    const data = await controlPlaneService.getAgents()
    const list = Array.isArray(data) ? data : (data.data || data.items || data.results || data.agents || [])
    agents.value = list
  } catch (err) {
    agents.value = []
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>

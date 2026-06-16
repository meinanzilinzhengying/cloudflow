<template>
  <div class="space-y-6">
    <!-- Page Header -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">Agent管理</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">管理和监控Agent节点</p>
      </div>
      <div class="flex items-center gap-3">
        <button class="btn-secondary">
          <Upload class="w-4 h-4" />
          批量升级
        </button>
        <button class="btn-primary">
          <Plus class="w-4 h-4" />
          添加Agent
        </button>
      </div>
    </div>
    <!-- Stats Cards -->
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
    <!-- Agent List -->
    <div class="card">
      <div class="p-6 border-b border-slate-200 dark:border-dark-700 flex items-center justify-between">
        <div class="flex items-center gap-3">
          <input type="text" placeholder="搜索Agent..." class="input max-w-xs" />
          <select class="input w-32">
            <option>全部状态</option>
            <option>在线</option>
            <option>离线</option>
            <option>异常</option>
          </select>
        </div>
        <button @click="fetchAgents" class="btn-secondary">
          <RefreshCw class="w-4 h-4" />
          刷新
        </button>
      </div>
      <div v-if="loading" class="p-8 text-center">
        <div class="animate-spin w-8 h-8 border-2 border-primary-500 border-t-transparent rounded-full mx-auto"></div>
        <p class="text-slate-500 mt-4">加载中...</p>
      </div>
      <div v-else class="overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="bg-slate-50 dark:bg-dark-700/50">
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">Agent名称</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">IP地址</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">版本</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">状态</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">运行时间</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr v-for="agent in agents" :key="agent.id">
              <td class="px-6 py-4">
                <div class="flex items-center gap-3">
                  <div :class="['w-3 h-3 rounded-full', agent.status === 'online' ? 'bg-green-500' : agent.status === 'offline' ? 'bg-gray-400' : 'bg-red-500']"></div>
                  <span class="text-sm font-medium text-slate-900 dark:text-white">{{ agent.name }}</span>
                </div>
              </td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ agent.ip }}</td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ agent.version }}</td>
              <td class="px-6 py-4">
                <span :class="['text-xs px-2 py-1 rounded-full font-medium', agent.status === 'online' ? 'bg-green-100 text-green-600' : agent.status === 'offline' ? 'bg-gray-100 text-gray-600' : 'bg-red-100 text-red-600']">
                  {{ agent.status === 'online' ? '在线' : agent.status === 'offline' ? '离线' : '异常' }}
                </span>
              </td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ agent.uptime }}</td>
              <td class="px-6 py-4">
                <div class="flex items-center gap-2">
                  <button @click="upgradeAgent(agent.id)" class="text-xs text-primary-500 hover:text-primary-600">升级</button>
                  <button @click="restartAgent(agent.id)" class="text-xs text-amber-500 hover:text-amber-600">重启</button>
                  <button @click="syncAgent(agent.id)" class="text-xs text-blue-500 hover:text-blue-600">同步</button>
                  <button @click="deleteAgent(agent.id)" class="text-xs text-red-500 hover:text-red-600">删除</button>
                </div>
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
import { Upload, Plus, Server, CheckCircle, AlertCircle, AlertTriangle, RefreshCw } from 'lucide-vue-next'
import { controlPlaneService } from '@/api'

const loading = ref(false)
const agentStats = ref({
  total: 0,
  online: 0,
  offline: 0,
  error: 0,
})
const agents = ref([])

const fetchAgents = async () => {
  loading.value = true
  try {
    const res = await controlPlaneService.getAgents()
    agents.value = res || []
    agentStats.value = {
      total: agents.value.length,
      online: agents.value.filter(a => a.status === 'online').length,
      offline: agents.value.filter(a => a.status === 'offline').length,
      error: agents.value.filter(a => a.status === 'error').length,
    }
  } catch (e) {
    console.error('Failed to fetch agents:', e)
  } finally {
    loading.value = false
  }
}

const upgradeAgent = async (id) => {
  try {
    await controlPlaneService.upgradeAgent(id, 'latest')
    fetchAgents()
  } catch (e) {
    console.error('Failed to upgrade agent:', e)
  }
}

const restartAgent = async (id) => {
  try {
    await controlPlaneService.restartAgent(id)
    fetchAgents()
  } catch (e) {
    console.error('Failed to restart agent:', e)
  }
}

const syncAgent = async (id) => {
  try {
    await controlPlaneService.pushConfig(id, {})
    fetchAgents()
  } catch (e) {
    console.error('Failed to sync agent:', e)
  }
}

const deleteAgent = async (id) => {
  if (confirm('确定要删除此Agent吗？')) {
    try {
      // await controlPlaneService.deleteAgent(id)
      fetchAgents()
    } catch (e) {
      console.error('Failed to delete agent:', e)
    }
  }
}

onMounted(() => {
  fetchAgents()
})
</script>
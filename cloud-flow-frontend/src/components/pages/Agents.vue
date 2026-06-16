<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">Agent 管理</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">管理 eBPF Agent 与 Edge 节点</p>
      </div>
      <div class="flex gap-2">
        <button @click="refresh" class="btn btn-secondary">
          <RefreshCw class="w-4 h-4 mr-2" />
          刷新
        </button>
      </div>
    </div>
    <!-- Agent Stats -->
    <div class="grid grid-cols-3 gap-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">在线 Agent</p>
        <p class="text-2xl font-bold text-green-500">{{ stats.online }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">离线 Agent</p>
        <p class="text-2xl font-bold text-slate-400">{{ stats.offline }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">Edge 节点</p>
        <p class="text-2xl font-bold text-blue-500">{{ stats.edges }}</p>
      </div>
    </div>
    <!-- Agent List -->
    <div class="card">
      <div v-if="loading" class="p-8 text-center">
        <div class="animate-spin w-8 h-8 border-2 border-primary-500 border-t-transparent rounded-full mx-auto"></div>
        <p class="text-slate-500 mt-4">加载中...</p>
      </div>
      <div v-else-if="agents.length === 0" class="p-8 text-center text-slate-500">
        <Server class="w-12 h-12 mx-auto mb-4 opacity-50" />
        <p>暂无 Agent 数据</p>
      </div>
      <div v-else class="overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="border-b border-slate-200 dark:border-dark-600">
              <th class="text-left p-4 text-sm font-medium text-slate-500">ID</th>
              <th class="text-left p-4 text-sm font-medium text-slate-500">名称</th>
              <th class="text-left p-4 text-sm font-medium text-slate-500">IP</th>
              <th class="text-left p-4 text-sm font-medium text-slate-500">状态</th>
              <th class="text-left p-4 text-sm font-medium text-slate-500">版本</th>
              <th class="text-left p-4 text-sm font-medium text-slate-500">最后心跳</th>
              <th class="text-left p-4 text-sm font-medium text-slate-500">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="agent in agents" :key="agent.id" class="border-b border-slate-100 dark:border-dark-700 hover:bg-slate-50 dark:hover:bg-dark-700/50">
              <td class="p-4 text-sm font-mono">{{ agent.id }}</td>
              <td class="p-4 text-sm">{{ agent.name }}</td>
              <td class="p-4 text-sm">{{ agent.ip }}</td>
              <td class="p-4">
                <span :class="['px-2 py-1 rounded-full text-xs font-medium', agent.status === 'online' ? 'bg-green-100 text-green-700 dark:bg-green-500/10 dark:text-green-400' : 'bg-slate-100 text-slate-600 dark:bg-slate-500/10 dark:text-slate-400']">
                  {{ agent.status === 'online' ? '在线' : '离线' }}
                </span>
              </td>
              <td class="p-4 text-sm">{{ agent.version }}</td>
              <td class="p-4 text-sm text-slate-500">{{ agent.last_heartbeat }}</td>
              <td class="p-4">
                <div class="flex gap-2">
                  <button @click="startAgent(agent.id)" class="btn btn-sm btn-success" :disabled="agent.status === 'online'">启动</button>
                  <button @click="stopAgent(agent.id)" class="btn btn-sm btn-danger" :disabled="agent.status !== 'online'">停止</button>
                  <button @click="restartAgent(agent.id)" class="btn btn-sm btn-secondary">重启</button>
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
import { Server, RefreshCw } from 'lucide-vue-next'
import { controlPlaneService } from '@/api'

const loading = ref(false)
const agents = ref([])
const stats = ref({ online: 0, offline: 0, edges: 0 })

const refresh = async () => {
  loading.value = true
  try {
    const res = await controlPlaneService.getAgents()
    agents.value = res || []
    stats.value = {
      online: agents.value.filter(a => a.status === 'online').length,
      offline: agents.value.filter(a => a.status !== 'online').length,
      edges: 0
    }
  } catch (e) {
    console.error('Failed to fetch agents:', e)
  } finally {
    loading.value = false
  }
}

const startAgent = async (id) => {
  try {
    await controlPlaneService.startAgent(id)
    refresh()
  } catch (e) {
    console.error('Failed to start agent:', e)
  }
}

const stopAgent = async (id) => {
  try {
    await controlPlaneService.stopAgent(id)
    refresh()
  } catch (e) {
    console.error('Failed to stop agent:', e)
  }
}

const restartAgent = async (id) => {
  try {
    await controlPlaneService.restartAgent(id)
    refresh()
  } catch (e) {
    console.error('Failed to restart agent:', e)
  }
}

onMounted(() => {
  refresh()
})
</script>
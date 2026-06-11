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
        <button class="btn-secondary">
          <RefreshCw class="w-4 h-4" />
          刷新
        </button>
      </div>
      <div class="overflow-x-auto">
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
                  <button class="text-xs text-primary-500 hover:text-primary-600">升级</button>
                  <button class="text-xs text-amber-500 hover:text-amber-600">重启</button>
                  <button class="text-xs text-blue-500 hover:text-blue-600">同步</button>
                  <button class="text-xs text-red-500 hover:text-red-600">删除</button>
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
import { ref } from 'vue'
import { Upload, Plus, Server, CheckCircle, AlertCircle, AlertTriangle, RefreshCw } from 'lucide-vue-next'

const agentStats = ref({
  total: 24,
  online: 20,
  offline: 2,
  error: 2,
})

const agents = ref([
  { id: 1, name: 'agent-01', ip: '192.168.1.10', version: 'v1.2.3', status: 'online', uptime: '5天' },
  { id: 2, name: 'agent-02', ip: '192.168.1.11', version: 'v1.2.3', status: 'online', uptime: '3天' },
  { id: 3, name: 'agent-03', ip: '192.168.1.12', version: 'v1.1.0', status: 'online', uptime: '10天' },
  { id: 4, name: 'agent-04', ip: '192.168.1.13', version: 'v1.2.3', status: 'offline', uptime: '-' },
  { id: 5, name: 'agent-05', ip: '192.168.1.14', version: 'v1.2.3', status: 'error', uptime: '2天' },
])
</script>

<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">TopN排行</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">流量排行分析</p>
      </div>
      <div class="flex items-center gap-3">
        <select class="input w-32">
          <option>按流量</option>
          <option>按包数</option>
          <option>按会话数</option>
        </select>
        <select class="input w-24">
          <option>Top 10</option>
          <option>Top 20</option>
          <option>Top 50</option>
        </select>
      </div>
    </div>

    <div class="grid grid-cols-2 gap-6">
      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">Top IP</h3>
        <div class="space-y-3">
          <div v-for="(item, i) in topIPs" :key="item.ip" class="flex items-center gap-3">
            <span class="w-6 text-xs font-medium text-slate-400">{{ i + 1 }}</span>
            <div class="flex-1">
              <div class="flex items-center justify-between mb-1">
                <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ item.ip }}</span>
                <span class="text-xs text-slate-500">{{ item.value }}</span>
              </div>
              <div class="h-2 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                <div class="h-full bg-gradient-to-r from-primary-500 to-accent-500 rounded-full" :style="{ width: `${item.percentage}%` }"></div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">Top 端口</h3>
        <div class="space-y-3">
          <div v-for="(item, i) in topPorts" :key="item.port" class="flex items-center gap-3">
            <span class="w-6 text-xs font-medium text-slate-400">{{ i + 1 }}</span>
            <div class="flex-1">
              <div class="flex items-center justify-between mb-1">
                <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ item.port }}/{{ item.protocol }}</span>
                <span class="text-xs text-slate-500">{{ item.value }}</span>
              </div>
              <div class="h-2 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                <div class="h-full bg-gradient-to-r from-accent-500 to-emerald-500 rounded-full" :style="{ width: `${item.percentage}%` }"></div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'

const topIPs = ref([
  { ip: '192.168.1.100', value: '2.1 GB', percentage: 100 },
  { ip: '192.168.1.101', value: '1.5 GB', percentage: 71 },
  { ip: '192.168.1.102', value: '980 MB', percentage: 47 },
  { ip: '192.168.1.103', value: '650 MB', percentage: 31 },
  { ip: '192.168.1.104', value: '420 MB', percentage: 20 },
])

const topPorts = ref([
  { port: 80, protocol: 'TCP', value: '3.2 GB', percentage: 100 },
  { port: 443, protocol: 'TCP', value: '2.1 GB', percentage: 66 },
  { port: 8080, protocol: 'TCP', value: '1.2 GB', percentage: 38 },
  { port: 3306, protocol: 'TCP', value: '850 MB', percentage: 27 },
  { port: 53, protocol: 'UDP', value: '420 MB', percentage: 13 },
])
</script>

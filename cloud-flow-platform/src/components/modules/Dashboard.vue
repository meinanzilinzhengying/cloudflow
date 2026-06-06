<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <h2 class="text-xl font-semibold text-white">平台概览</h2>
      <div class="flex items-center gap-2">
        <span class="text-sm text-gray-400">最后更新: {{ lastUpdate }}</span>
        <button @click="refreshData" class="p-2 hover:bg-dark-100 rounded-lg transition-colors">
          <RefreshCw class="w-4 h-4 text-gray-400" :class="{ 'animate-spin': loading }" />
        </button>
      </div>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
      <StatCard
        title="CPU 使用率"
        :value="`${stats.cpu?.usage || 0}%`"
        subtitle="8 核心"
        icon="Cpu"
        :percent="stats.cpu?.usage"
      />
      <StatCard
        title="内存使用"
        :value="`${stats.memory?.used || 0} / ${stats.memory?.total || 0} GB`"
        subtitle="DDR4 3200MHz"
        icon="HardDrive"
        :percent="stats.memory?.usage"
      />
      <StatCard
        title="磁盘使用"
        :value="`${stats.disk?.used || 0} / ${stats.disk?.total || 0} GB`"
        subtitle="SSD NVMe"
        icon="Database"
        :percent="stats.disk?.usage"
      />
      <StatCard
        title="网络流量"
        :value="`↓ ${stats.network?.inbound || 0} / ↑ ${stats.network?.outbound || 0} MB/s`"
        subtitle="千兆网卡"
        icon="Network"
      />
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <TrendChart
        title="CPU 使用率趋势"
        subtitle="过去 24 小时"
        :labels="cpuLabels"
        :data="cpuData"
        color="#89b4fa"
      />
      <TrendChart
        title="内存使用趋势"
        subtitle="过去 24 小时"
        :labels="memLabels"
        :data="memData"
        color="#f5c2e7"
      />
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-3 gap-4">
      <StatCard
        title="服务状态"
        :value="`${stats.services?.running || 0} / ${stats.services?.total || 0}`"
        subtitle="运行中"
        icon="Server"
      />
      <StatCard
        title="平台运行时长"
        :value="uptimeFormatted"
        subtitle="最后启动: 10天前"
        icon="Clock"
      />
      <StatCard
        title="告警统计"
        :value="`${alertsCount.firing} / ${alertsCount.total}`"
        subtitle="触发中 / 总计"
        icon="Bell"
      />
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import StatCard from '../common/StatCard.vue'
import TrendChart from '../common/TrendChart.vue'
import api from '../../api'
import { RefreshCw } from 'lucide-vue-next'

const loading = ref(false)
const stats = ref({})
const lastUpdate = ref('')

const alertsCount = ref({ firing: 2, total: 12 })

const cpuLabels = Array.from({ length: 24 }, (_, i) => `${i}:00`)
const cpuData = Array.from({ length: 24 }, () => Math.floor(Math.random() * 30 + 30))

const memLabels = Array.from({ length: 24 }, (_, i) => `${i}:00`)
const memData = Array.from({ length: 24 }, () => Math.floor(Math.random() * 20 + 40))

const uptimeFormatted = computed(() => {
  const seconds = stats.value.uptime || 864000
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  return `${days}天 ${hours}小时`
})

async function refreshData() {
  loading.value = true
  try {
    stats.value = await api.getPlatformStats()
    lastUpdate.value = new Date().toLocaleTimeString('zh-CN')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  refreshData()
})
</script>

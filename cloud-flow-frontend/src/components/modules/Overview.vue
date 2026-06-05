<template>
  <div>
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
      <StatCard
        title="总流量"
        :value="stats.totalFlows"
        unit="flows"
        :change="stats.flowsChange"
        :icon="ArrowRightLeft"
        variant="default"
        :loading="loading"
      />
      <StatCard
        title="活跃连接"
        :value="stats.activeConnections"
        unit="conn"
        :change="stats.connChange"
        :icon="Link"
        variant="info"
        :loading="loading"
      />
      <StatCard
        title="重传率"
        :value="stats.retransRate"
        unit="%"
        :change="stats.retransChange"
        :icon="RefreshCw"
        :variant="stats.retransRate > 5 ? 'danger' : 'success'"
        :loading="loading"
      />
      <StatCard
        title="丢包率"
        :value="stats.packetLoss"
        unit="%"
        :change="stats.lossChange"
        :icon="AlertTriangle"
        :variant="stats.packetLoss > 1 ? 'danger' : 'success'"
        :loading="loading"
      />
    </div>
    
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <TrendChart
        title="流量趋势"
        subtitle="过去 6 小时"
        type="line"
        :data="trafficChartData"
        :legends="trafficLegends"
      />
      <TrendChart
        title="协议分布"
        type="doughnut"
        :data="protocolChartData"
        :legends="protocolLegends"
        :options="doughnutOptions"
      />
    </div>
    
    <div class="mt-6">
      <div class="flex items-center justify-between mb-4">
        <h3 class="font-semibold text-white">Top 流量来源</h3>
        <button class="text-sm text-primary-400 hover:text-primary-300 transition">查看全部</button>
      </div>
      <div class="bg-dark-800 rounded-xl border border-dark-600 overflow-hidden">
        <table class="w-full">
          <thead>
            <tr class="border-b border-dark-600">
              <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">源 IP</th>
              <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">协议</th>
              <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">流量</th>
              <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">占比</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in topFlows" :key="item.source" class="border-b border-dark-700 hover:bg-dark-700/50 transition">
              <td class="px-4 py-3 text-sm text-white">{{ item.source }}</td>
              <td class="px-4 py-3">
                <span 
                  class="px-2 py-1 text-xs rounded-full"
                  :class="getProtocolClass(item.protocol)"
                >
                  {{ item.protocol }}
                </span>
              </td>
              <td class="px-4 py-3 text-sm text-gray-300">{{ formatBytes(item.bytes) }}</td>
              <td class="px-4 py-3">
                <div class="flex items-center gap-2">
                  <div class="flex-1 h-2 bg-dark-700 rounded-full overflow-hidden">
                    <div 
                      class="h-full bg-primary-500 rounded-full transition-all"
                      :style="{ width: item.percentage + '%' }"
                    ></div>
                  </div>
                  <span class="text-xs text-gray-400 w-12 text-right">{{ item.percentage }}%</span>
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
import { ref, onMounted, computed } from 'vue'
import { ArrowRightLeft, Link, RefreshCw, AlertTriangle } from 'lucide-vue-next'
import StatCard from '../common/StatCard.vue'
import TrendChart from '../common/TrendChart.vue'
import { overviewApi, trafficApi } from '../../api'

const loading = ref(true)
const stats = ref({
  totalFlows: 0,
  flowsChange: '+12%',
  activeConnections: 0,
  connChange: '+5%',
  retransRate: 0,
  retransChange: '-2%',
  packetLoss: 0,
  lossChange: '-0.5%'
})

const topFlows = ref([
  { source: '10.10.1.25', protocol: 'TCP', bytes: 125000000, percentage: 25 },
  { source: '10.10.1.36', protocol: 'HTTP', bytes: 98000000, percentage: 20 },
  { source: '10.10.1.12', protocol: 'HTTPS', bytes: 75000000, percentage: 15 },
  { source: '10.10.1.89', protocol: 'UDP', bytes: 50000000, percentage: 10 },
  { source: '10.10.1.45', protocol: 'DNS', bytes: 30000000, percentage: 6 }
])

const trafficChartData = computed(() => ({
  labels: ['00:00', '02:00', '04:00', '06:00', '08:00', '10:00', '12:00'],
  datasets: [
    {
      label: '入站流量',
      data: [1200, 1900, 1500, 2200, 1800, 2500, 2100],
      borderColor: '#3b82f6',
      backgroundColor: 'rgba(59, 130, 246, 0.1)',
      fill: true,
      tension: 0.4
    },
    {
      label: '出站流量',
      data: [800, 1200, 900, 1500, 1100, 1800, 1400],
      borderColor: '#10b981',
      backgroundColor: 'rgba(16, 185, 129, 0.1)',
      fill: true,
      tension: 0.4
    }
  ]
}))

const trafficLegends = [
  { label: '入站流量', color: '#3b82f6' },
  { label: '出站流量', color: '#10b981' }
]

const protocolChartData = computed(() => ({
  labels: ['TCP', 'HTTP', 'HTTPS', 'UDP', 'DNS', '其他'],
  datasets: [{
    data: [35, 25, 20, 12, 5, 3],
    backgroundColor: [
      '#3b82f6',
      '#10b981',
      '#8b5cf6',
      '#f59e0b',
      '#ec4899',
      '#6b7280'
    ],
    borderWidth: 0
  }]
}))

const protocolLegends = [
  { label: 'TCP', color: '#3b82f6' },
  { label: 'HTTP', color: '#10b981' },
  { label: 'HTTPS', color: '#8b5cf6' },
  { label: 'UDP', color: '#f59e0b' },
  { label: 'DNS', color: '#ec4899' },
  { label: '其他', color: '#6b7280' }
]

const doughnutOptions = {
  cutout: '65%',
  plugins: {
    legend: {
      display: false
    }
  }
}

const getProtocolClass = (protocol) => {
  const classes = {
    'TCP': 'bg-blue-500/20 text-blue-400',
    'HTTP': 'bg-green-500/20 text-green-400',
    'HTTPS': 'bg-purple-500/20 text-purple-400',
    'UDP': 'bg-yellow-500/20 text-yellow-400',
    'DNS': 'bg-pink-500/20 text-pink-400'
  }
  return classes[protocol] || 'bg-gray-500/20 text-gray-400'
}

const formatBytes = (bytes) => {
  if (bytes >= 1000000) return (bytes / 1000000).toFixed(1) + ' MB'
  if (bytes >= 1000) return (bytes / 1000).toFixed(1) + ' KB'
  return bytes + ' B'
}

onMounted(async () => {
  try {
    const data = await overviewApi.getStats()
    if (data) {
      stats.value = {
        totalFlows: data.totalFlows || 1250000,
        flowsChange: data.flowsChange || '+12%',
        activeConnections: data.activeConnections || 8450,
        connChange: data.connChange || '+5%',
        retransRate: data.retransRate || 3.2,
        retransChange: data.retransChange || '-2%',
        packetLoss: data.packetLoss || 0.3,
        lossChange: data.lossChange || '-0.5%'
      }
    }
  } catch (error) {
    console.error('Failed to fetch overview:', error)
  } finally {
    loading.value = false
  }
})
</script>

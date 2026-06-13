<template>
  <div>
    <div class="flex items-center gap-4 mb-6">
      <div class="flex items-center gap-2 bg-dark-800 px-4 py-2 rounded-lg border border-dark-600">
        <span class="text-gray-400 text-sm">协议:</span>
        <select 
          v-model="selectedProtocol" 
          class="bg-dark-700 border-none text-white text-sm px-3 py-1 rounded-md focus:outline-none"
        >
          <option value="all">全部协议</option>
          <option v-for="p in protocols" :key="p" :value="p">{{ p }}</option>
        </select>
      </div>
      <div class="flex items-center gap-2 bg-dark-800 px-4 py-2 rounded-lg border border-dark-600">
        <span class="text-gray-400 text-sm">方向:</span>
        <select 
          v-model="selectedDirection" 
          class="bg-dark-700 border-none text-white text-sm px-3 py-1 rounded-md focus:outline-none"
        >
          <option value="both">双向</option>
          <option value="inbound">入站</option>
          <option value="outbound">出站</option>
        </select>
      </div>
    </div>
    
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <div class="lg:col-span-2">
        <TrendChart
          title="实时流量"
          subtitle="数据包/秒"
          type="line"
          :data="realtimeChartData"
          :legends="realtimeLegends"
        />
      </div>
      <div>
        <TrendChart
          title="连接状态"
          type="bar"
          :data="connectionChartData"
          :legends="connectionLegends"
          :options="barOptions"
        />
      </div>
    </div>
    
    <div class="mt-6">
      <div class="flex items-center justify-between mb-4">
        <h3 class="font-semibold text-white">流量明细</h3>
        <div class="flex gap-2">
          <button 
            v-for="tab in detailTabs" 
            :key="tab.value"
            @click="activeDetailTab = tab.value"
            :class="[
              'px-4 py-2 text-sm font-medium rounded-lg transition',
              activeDetailTab === tab.value
                ? 'bg-primary-500 text-white'
                : 'bg-dark-700 text-gray-400 hover:text-white'
            ]"
          >
            {{ tab.label }}
          </button>
        </div>
      </div>
      
      <div class="bg-dark-800 rounded-xl border border-dark-600 overflow-hidden">
        <table class="w-full">
          <thead>
            <tr class="border-b border-dark-600">
              <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">时间</th>
              <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">源 IP:端口</th>
              <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">目的 IP:端口</th>
              <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">协议</th>
              <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">流量</th>
              <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">状态</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="flow in flowDetails" :key="flow.id" class="border-b border-dark-700 hover:bg-dark-700/50 transition">
              <td class="px-4 py-3 text-sm text-gray-400">{{ flow.time }}</td>
              <td class="px-4 py-3 text-sm text-white">{{ flow.source }}</td>
              <td class="px-4 py-3 text-sm text-white">{{ flow.destination }}</td>
              <td class="px-4 py-3">
                <span 
                  class="px-2 py-1 text-xs rounded-full"
                  :class="getProtocolClass(flow.protocol)"
                >
                  {{ flow.protocol }}
                </span>
              </td>
              <td class="px-4 py-3 text-sm text-gray-300">{{ formatBytes(flow.bytes) }}</td>
              <td class="px-4 py-3">
                <span 
                  class="w-2 h-2 rounded-full inline-block"
                  :class="flow.status === 'active' ? 'bg-green-500' : 'bg-gray-500'"
                ></span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import TrendChart from '../common/TrendChart.vue'
import { trafficApi } from '../../api'

const selectedProtocol = ref('all')
const selectedDirection = ref('both')
const activeDetailTab = ref('all')

const protocols = ref(['TCP', 'HTTP', 'HTTPS', 'UDP', 'DNS', 'gRPC', 'Kafka'])

const detailTabs = [
  { label: '全部', value: 'all' },
  { label: '活跃', value: 'active' },
  { label: '最近', value: 'recent' }
]

const flowDetails = ref([])

const realtimeChartData = computed(() => ({
  labels: ['0s', '10s', '20s', '30s', '40s', '50s', '60s'],
  datasets: [
    {
      label: '入站',
      data: [1200, 1500, 1300, 1800, 1600, 1900, 1700],
      borderColor: '#3b82f6',
      backgroundColor: 'rgba(59, 130, 246, 0.1)',
      fill: true,
      tension: 0.4
    },
    {
      label: '出站',
      data: [800, 1000, 900, 1200, 1100, 1300, 1200],
      borderColor: '#10b981',
      backgroundColor: 'rgba(16, 185, 129, 0.1)',
      fill: true,
      tension: 0.4
    }
  ]
}))

const realtimeLegends = [
  { label: '入站', color: '#3b82f6' },
  { label: '出站', color: '#10b981' }
]

const connectionChartData = computed(() => ({
  labels: ['SYN', 'ESTABLISHED', 'FIN', 'CLOSE'],
  datasets: [{
    data: [150, 850, 80, 120],
    backgroundColor: [
      '#f59e0b',
      '#10b981',
      '#ec4899',
      '#6b7280'
    ],
    borderWidth: 0
  }]
}))

const connectionLegends = [
  { label: 'SYN', color: '#f59e0b' },
  { label: 'ESTABLISHED', color: '#10b981' },
  { label: 'FIN', color: '#ec4899' },
  { label: 'CLOSE', color: '#6b7280' }
]

const barOptions = {
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
    'DNS': 'bg-pink-500/20 text-pink-400',
    'gRPC': 'bg-cyan-500/20 text-cyan-400',
    'Kafka': 'bg-orange-500/20 text-orange-400'
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
    const data = await trafficApi.getProtocols()
    if (data && data.protocols) {
      protocols.value = data.protocols
    }
  } catch (error) {
    console.error('Failed to fetch protocols:', error)
  }
})
</script>

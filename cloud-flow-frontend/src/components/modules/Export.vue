<template>
  <div>
    <div class="bg-dark-800 rounded-xl p-6 border border-dark-600 mb-6">
      <h3 class="font-semibold text-white mb-4">导出设置</h3>
      
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
        <div>
          <label class="block text-gray-400 text-sm mb-2">导出格式</label>
          <select 
            v-model="exportFormat" 
            class="w-full bg-dark-700 border border-dark-600 rounded-lg px-4 py-2.5 text-white focus:outline-none focus:border-primary-500"
          >
            <option value="json">JSON</option>
            <option value="csv">CSV</option>
          </select>
        </div>
        <div>
          <label class="block text-gray-400 text-sm mb-2">时间范围</label>
          <select 
            v-model="exportTimeRange" 
            class="w-full bg-dark-700 border border-dark-600 rounded-lg px-4 py-2.5 text-white focus:outline-none focus:border-primary-500"
          >
            <option value="1h">最近 1 小时</option>
            <option value="6h">最近 6 小时</option>
            <option value="24h">最近 24 小时</option>
            <option value="7d">最近 7 天</option>
            <option value="custom">自定义时间</option>
          </select>
        </div>
      </div>
      
      <div v-if="exportTimeRange === 'custom'" class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
        <div>
          <label class="block text-gray-400 text-sm mb-2">开始时间</label>
          <input 
            type="datetime-local" 
            v-model="startTime" 
            class="w-full bg-dark-700 border border-dark-600 rounded-lg px-4 py-2.5 text-white focus:outline-none focus:border-primary-500"
          />
        </div>
        <div>
          <label class="block text-gray-400 text-sm mb-2">结束时间</label>
          <input 
            type="datetime-local" 
            v-model="endTime" 
            class="w-full bg-dark-700 border border-dark-600 rounded-lg px-4 py-2.5 text-white focus:outline-none focus:border-primary-500"
          />
        </div>
      </div>
      
      <div class="mb-6">
        <label class="block text-gray-400 text-sm mb-2">筛选条件（可选）</label>
        <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
          <select 
            v-model="filterProtocol" 
            class="bg-dark-700 border border-dark-600 rounded-lg px-4 py-2.5 text-white focus:outline-none focus:border-primary-500"
          >
            <option value="">全部协议</option>
            <option v-for="p in protocols" :key="p" :value="p">{{ p }}</option>
          </select>
          <select 
            v-model="filterNamespace" 
            class="bg-dark-700 border border-dark-600 rounded-lg px-4 py-2.5 text-white focus:outline-none focus:border-primary-500"
          >
            <option value="">全部 Namespace</option>
            <option v-for="ns in namespaces" :key="ns" :value="ns">{{ ns }}</option>
          </select>
          <input 
            type="text" 
            v-model="filterIP" 
            placeholder="过滤 IP 地址"
            class="bg-dark-700 border border-dark-600 rounded-lg px-4 py-2.5 text-white placeholder-gray-500 focus:outline-none focus:border-primary-500"
          />
        </div>
      </div>
      
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-4">
          <div class="flex items-center gap-2">
            <input 
              type="checkbox" 
              v-model="includeHeaders" 
              id="includeHeaders"
              class="w-4 h-4 rounded bg-dark-700 border-dark-600 text-primary-500 focus:ring-primary-500"
            />
            <label for="includeHeaders" class="text-gray-400 text-sm">包含表头（CSV）</label>
          </div>
          <div class="flex items-center gap-2">
            <input 
              type="checkbox" 
              v-model="compress" 
              id="compress"
              class="w-4 h-4 rounded bg-dark-700 border-dark-600 text-primary-500 focus:ring-primary-500"
            />
            <label for="compress" class="text-gray-400 text-sm">压缩文件（.zip）</label>
          </div>
        </div>
        <button 
          @click="handleExport"
          :disabled="isExporting"
          class="px-6 py-2.5 bg-primary-500 text-white font-medium rounded-lg hover:bg-primary-600 transition disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
        >
          <Loader2 v-if="isExporting" class="w-4 h-4 animate-spin" />
          <Download v-else class="w-4 h-4" />
          {{ isExporting ? '导出中...' : '开始导出' }}
        </button>
      </div>
    </div>
    
    <div class="bg-dark-800 rounded-xl border border-dark-600 overflow-hidden">
      <div class="p-4 border-b border-dark-600">
        <h3 class="font-semibold text-white">导出历史</h3>
      </div>
      <div class="max-h-[300px] overflow-y-auto">
        <div 
          v-for="history in exportHistory" 
          :key="history.id"
          class="px-4 py-3 border-b border-dark-700 hover:bg-dark-700/50 transition flex items-center justify-between"
        >
          <div class="flex items-center gap-3">
            <div 
              class="w-10 h-10 rounded-lg flex items-center justify-center"
              :class="history.status === 'completed' ? 'bg-green-500/20' : 'bg-yellow-500/20'"
            >
              <FileDown class="w-5 h-5" :class="history.status === 'completed' ? 'text-green-400' : 'text-yellow-400'" />
            </div>
            <div>
              <p class="text-white text-sm font-medium">{{ history.filename }}</p>
              <p class="text-gray-500 text-xs">{{ history.time }} · {{ history.size }}</p>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <button 
              v-if="history.status === 'completed'"
              class="px-3 py-1.5 bg-dark-700 text-gray-400 text-xs font-medium rounded-md hover:bg-dark-600 transition"
            >
              下载
            </button>
            <span 
              class="text-xs px-2 py-1 rounded-full"
              :class="history.status === 'completed' ? 'bg-green-500/20 text-green-400' : 'bg-yellow-500/20 text-yellow-400'"
            >
              {{ history.status === 'completed' ? '已完成' : '处理中' }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { Download, Loader2, FileDown } from 'lucide-vue-next'
import { exportApi, filtersApi } from '../../api'

const exportFormat = ref('json')
const exportTimeRange = ref('24h')
const startTime = ref('')
const endTime = ref('')
const filterProtocol = ref('')
const filterNamespace = ref('')
const filterIP = ref('')
const includeHeaders = ref(true)
const compress = ref(false)
const isExporting = ref(false)

const protocols = ref(['TCP', 'HTTP', 'HTTPS', 'UDP', 'DNS'])
const namespaces = ref(['default', 'cloudflow', 'monitoring'])

const exportHistory = ref([
  { id: 1, filename: 'traffic_data_20240115.json', time: '10分钟前', size: '2.3 MB', status: 'completed' },
  { id: 2, filename: 'network_analysis_20240114.csv', time: '1小时前', size: '5.8 MB', status: 'completed' },
  { id: 3, filename: 'alerts_report_20240114.json', time: '3小时前', size: '1.2 MB', status: 'completed' },
  { id: 4, filename: 'full_export_20240113.zip', time: '1天前', size: '45.6 MB', status: 'completed' }
])

const handleExport = async () => {
  isExporting.value = true
  
  try {
    const params = {
      format: exportFormat.value,
      time_range: exportTimeRange.value
    }
    
    if (exportTimeRange.value === 'custom') {
      params.start_time = startTime.value
      params.end_time = endTime.value
    }
    
    if (filterProtocol.value) params.protocol = filterProtocol.value
    if (filterNamespace.value) params.namespace = filterNamespace.value
    if (filterIP.value) params.ip = filterIP.value
    if (includeHeaders.value) params.include_headers = 'true'
    if (compress.value) params.compress = 'true'
    
    const blob = await exportApi.exportData(params)
    
    const filename = `cloudflow_export_${new Date().toISOString().slice(0, 10)}.${exportFormat.value}`
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    window.URL.revokeObjectURL(url)
    
    exportHistory.value.unshift({
      id: Date.now(),
      filename,
      time: '刚刚',
      size: `${(blob.size / 1024).toFixed(1)} KB`,
      status: 'completed'
    })
  } catch (error) {
    console.error('Export failed:', error)
    alert('导出失败，请重试')
  } finally {
    isExporting.value = false
  }
}

onMounted(async () => {
  try {
    const [protocolsData, namespacesData] = await Promise.all([
      filtersApi.getProtocols(),
      filtersApi.getNamespaces()
    ])
    
    if (protocolsData && protocolsData.protocols) {
      protocols.value = protocolsData.protocols
    }
    
    if (namespacesData && namespacesData.namespaces) {
      namespaces.value = namespacesData.namespaces
    }
  } catch (error) {
    console.error('Failed to fetch filters:', error)
  }
})
</script>

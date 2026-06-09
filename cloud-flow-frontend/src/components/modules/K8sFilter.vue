<template>
  <div>
    <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
      <div>
        <label class="block text-gray-400 text-sm mb-2">Namespace</label>
        <select 
          v-model="selectedNamespace" 
          @change="handleNamespaceChange"
          class="w-full bg-dark-700 border border-dark-600 rounded-lg px-4 py-2.5 text-white focus:outline-none focus:border-primary-500"
        >
          <option value="">选择 Namespace</option>
          <option v-for="ns in namespaces" :key="ns" :value="ns">{{ ns }}</option>
        </select>
      </div>
      <div>
        <label class="block text-gray-400 text-sm mb-2">Service</label>
        <select 
          v-model="selectedService" 
          @change="handleServiceChange"
          class="w-full bg-dark-700 border border-dark-600 rounded-lg px-4 py-2.5 text-white focus:outline-none focus:border-primary-500"
        >
          <option value="">选择 Service</option>
          <option v-for="svc in services" :key="svc" :value="svc">{{ svc }}</option>
        </select>
      </div>
      <div>
        <label class="block text-gray-400 text-sm mb-2">Pod</label>
        <select 
          v-model="selectedPod" 
          class="w-full bg-dark-700 border border-dark-600 rounded-lg px-4 py-2.5 text-white focus:outline-none focus:border-primary-500"
        >
          <option value="">选择 Pod</option>
          <option v-for="pod in pods" :key="pod" :value="pod">{{ pod }}</option>
        </select>
      </div>
    </div>
    
    <div class="flex items-center justify-between mb-4">
      <h3 class="font-semibold text-white">资源列表</h3>
      <div class="flex gap-2">
        <button 
          v-for="tab in resourceTabs" 
          :key="tab.value"
          @click="activeResourceTab = tab.value"
          :class="[
            'px-4 py-2 text-sm font-medium rounded-lg transition',
            activeResourceTab === tab.value
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
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">名称</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">状态</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">CPU</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">内存</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">网络</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in resourceList" :key="item.name" class="border-b border-dark-700 hover:bg-dark-700/50 transition">
            <td class="px-4 py-3">
              <div class="flex items-center gap-3">
                <component :is="item.icon" class="w-5 h-5 text-gray-400" />
                <div>
                  <p class="text-white text-sm font-medium">{{ item.name }}</p>
                  <p class="text-gray-500 text-xs">{{ item.namespace }}</p>
                </div>
              </div>
            </td>
            <td class="px-4 py-3">
              <span 
                class="px-2 py-1 text-xs rounded-full"
                :class="getStatusClass(item.status)"
              >
                {{ item.status }}
              </span>
            </td>
            <td class="px-4 py-3">
              <div class="flex items-center gap-2">
                <div class="w-24 h-2 bg-dark-700 rounded-full overflow-hidden">
                  <div 
                    class="h-full rounded-full"
                    :class="getUsageClass(item.cpu)"
                    :style="{ width: item.cpu + '%' }"
                  ></div>
                </div>
                <span class="text-xs text-gray-400">{{ item.cpu }}%</span>
              </div>
            </td>
            <td class="px-4 py-3">
              <div class="flex items-center gap-2">
                <div class="w-24 h-2 bg-dark-700 rounded-full overflow-hidden">
                  <div 
                    class="h-full rounded-full"
                    :class="getUsageClass(item.memory)"
                    :style="{ width: item.memory + '%' }"
                  ></div>
                </div>
                <span class="text-xs text-gray-400">{{ item.memory }}%</span>
              </div>
            </td>
            <td class="px-4 py-3">
              <span class="text-xs text-gray-400">{{ formatBytes(item.network) }}/s</span>
            </td>
            <td class="px-4 py-3">
              <div class="flex gap-1">
                <button class="p-1.5 hover:bg-dark-700 rounded transition" title="查看详情">
                  <Eye class="w-4 h-4 text-gray-400" />
                </button>
                <button class="p-1.5 hover:bg-dark-700 rounded transition" title="日志">
                  <FileText class="w-4 h-4 text-gray-400" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { Server, Eye, FileText } from 'lucide-vue-next'
import { filtersApi } from '../../api'

const selectedNamespace = ref('')
const selectedService = ref('')
const selectedPod = ref('')
const activeResourceTab = ref('pods')

const namespaces = ref(['default', 'kube-system', 'cloudflow', 'monitoring'])
const services = ref([])
const pods = ref([])

const resourceTabs = [
  { label: 'Pods', value: 'pods' },
  { label: 'Services', value: 'services' },
  { label: 'Deployments', value: 'deployments' }
]

const resourceList = ref([
  { name: 'cloudflow-agent-abc12', namespace: 'cloudflow', status: 'Running', cpu: 45, memory: 62, network: 125000, icon: Server },
  { name: 'cloudflow-center-def34', namespace: 'cloudflow', status: 'Running', cpu: 32, memory: 45, network: 89000, icon: Server },
  { name: 'cloudflow-edge-ghi56', namespace: 'cloudflow', status: 'Running', cpu: 58, memory: 71, network: 230000, icon: Server },
  { name: 'redis-master-jkl78', namespace: 'default', status: 'Running', cpu: 12, memory: 28, network: 45000, icon: Server },
  { name: 'clickhouse-server-mno90', namespace: 'default', status: 'Running', cpu: 78, memory: 85, network: 450000, icon: Server },
  { name: 'prometheus-server-pqr11', namespace: 'monitoring', status: 'Running', cpu: 25, memory: 55, network: 150000, icon: Server }
])

const getStatusClass = (status) => {
  const classes = {
    'Running': 'bg-green-500/20 text-green-400',
    'Pending': 'bg-yellow-500/20 text-yellow-400',
    'Failed': 'bg-red-500/20 text-red-400',
    'Unknown': 'bg-gray-500/20 text-gray-400'
  }
  return classes[status] || 'bg-gray-500/20 text-gray-400'
}

const getUsageClass = (usage) => {
  if (usage >= 80) return 'bg-red-500'
  if (usage >= 60) return 'bg-yellow-500'
  return 'bg-green-500'
}

const formatBytes = (bytes) => {
  if (bytes >= 1000000) return (bytes / 1000000).toFixed(1) + ' MB'
  if (bytes >= 1000) return (bytes / 1000).toFixed(1) + ' KB'
  return bytes + ' B'
}

const handleNamespaceChange = async () => {
  if (!selectedNamespace.value) {
    services.value = []
    selectedService.value = ''
    pods.value = []
    selectedPod.value = ''
    return
  }
  
  try {
    const data = await filtersApi.getServices(selectedNamespace.value)
    services.value = data.services || []
    selectedService.value = ''
    pods.value = []
    selectedPod.value = ''
  } catch (error) {
    console.error('Failed to fetch services:', error)
  }
}

const handleServiceChange = async () => {
  if (!selectedService.value) {
    pods.value = []
    selectedPod.value = ''
    return
  }
  
  try {
    const data = await filtersApi.getPods(selectedNamespace.value, selectedService.value)
    pods.value = data.pods || []
    selectedPod.value = ''
  } catch (error) {
    console.error('Failed to fetch pods:', error)
  }
}

onMounted(async () => {
  try {
    const data = await filtersApi.getNamespaces()
    if (data && data.namespaces) {
      namespaces.value = data.namespaces
    }
  } catch (error) {
    console.error('Failed to fetch namespaces:', error)
  }
})
</script>

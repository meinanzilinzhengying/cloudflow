<template>
  <div>
    <div class="flex items-center justify-between mb-6">
      <div class="flex items-center gap-4">
        <div class="flex items-center gap-2 bg-dark-800 px-4 py-2 rounded-lg border border-dark-600">
          <span class="text-gray-400 text-sm">类型:</span>
          <select 
            v-model="selectedType" 
            class="bg-dark-700 border-none text-white text-sm px-3 py-1 rounded-md focus:outline-none"
          >
            <option value="all">全部类型</option>
            <option value="agent">Agent</option>
            <option value="center">Center</option>
            <option value="edge">Edge</option>
          </select>
        </div>
        <div class="flex items-center gap-2 bg-dark-800 px-4 py-2 rounded-lg border border-dark-600">
          <span class="text-gray-400 text-sm">分组:</span>
          <select 
            v-model="selectedGroup" 
            class="bg-dark-700 border-none text-white text-sm px-3 py-1 rounded-md focus:outline-none"
          >
            <option value="all">全部分组</option>
            <option v-for="group in groups" :key="group" :value="group">{{ group }}</option>
          </select>
        </div>
      </div>
      <div class="flex gap-3">
        <button @click="showGroupModal = true" class="px-4 py-2 bg-dark-700 text-white text-sm font-medium rounded-lg hover:bg-dark-600 transition">
          分组管理
        </button>
        <button @click="showInstallModal = true" class="px-4 py-2 bg-primary-500 text-white text-sm font-medium rounded-lg hover:bg-primary-600 transition flex items-center gap-2">
          <Download class="w-4 h-4" />
          安装探针
        </button>
      </div>
    </div>
    
    <div class="bg-dark-800 rounded-xl border border-dark-600 overflow-hidden">
      <div class="px-4 py-3 border-b border-dark-600">
        <h3 class="font-semibold text-white">探针列表</h3>
      </div>
      <table class="w-full">
        <thead>
          <tr class="border-b border-dark-600">
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">探针名称</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">类型</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">分组</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">版本</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">状态</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">最后心跳</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="probe in filteredProbes" :key="probe.id" class="border-b border-dark-700 hover:bg-dark-700/50 transition">
            <td class="px-4 py-3 text-sm text-white">{{ probe.name }}</td>
            <td class="px-4 py-3 text-sm text-gray-300">{{ probe.type }}</td>
            <td class="px-4 py-3 text-sm text-gray-300">{{ probe.group }}</td>
            <td class="px-4 py-3 text-sm text-gray-300">{{ probe.version }}</td>
            <td class="px-4 py-3">
              <span 
                class="px-2 py-1 text-xs rounded-full"
                :class="probe.status === 'online' ? 'bg-green-500/20 text-green-400' : 'bg-red-500/20 text-red-400'"
              >
                {{ probe.status === 'online' ? '在线' : '离线' }}
              </span>
            </td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ probe.lastHeartbeat }}</td>
            <td class="px-4 py-3">
              <div class="flex gap-2">
                <button @click="upgradeProbe(probe)" class="p-1.5 hover:bg-dark-600 rounded text-primary-400" title="升级">
                  <ArrowUpCircle class="w-4 h-4" />
                </button>
                <button @click="uninstallProbe(probe)" class="p-1.5 hover:bg-dark-600 rounded text-red-400" title="卸载">
                  <Trash2 class="w-4 h-4" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    
    <!-- 安装弹窗 -->
    <div v-if="showInstallModal" class="fixed inset-0 bg-dark-900/80 flex items-center justify-center z-50">
      <div class="bg-dark-800 rounded-xl p-6 w-full max-w-lg border border-dark-600">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-semibold text-white">安装新探针</h3>
          <button @click="showInstallModal = false" class="text-gray-400 hover:text-white">
            <X class="w-5 h-5" />
          </button>
        </div>
        <div class="space-y-4">
          <div>
            <label class="block text-sm text-gray-400 mb-1">探针名称</label>
            <input v-model="newProbe.name" type="text" class="w-full px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500" placeholder="输入探针名称" />
          </div>
          <div>
            <label class="block text-sm text-gray-400 mb-1">探针类型</label>
            <select v-model="newProbe.type" class="w-full px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500">
              <option value="agent">Agent</option>
              <option value="center">Center</option>
              <option value="edge">Edge</option>
            </select>
          </div>
          <div>
            <label class="block text-sm text-gray-400 mb-1">分组</label>
            <input v-model="newProbe.group" type="text" class="w-full px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500" placeholder="输入分组名称" />
          </div>
          <div>
            <label class="block text-sm text-gray-400 mb-2">安装命令</label>
            <div class="p-3 bg-dark-700 rounded-lg text-sm font-mono text-primary-400 select-all">
              curl -sSL https://install.cloudflow.io/probe.sh | sh -s -- --name {{ newProbe.name || 'YOUR_PROBE_NAME' }} --type {{ newProbe.type || 'agent' }}
            </div>
          </div>
        </div>
        <div class="flex justify-end gap-3 mt-6">
          <button @click="showInstallModal = false" class="px-4 py-2 text-gray-400 hover:text-white transition">取消</button>
          <button @click="installProbe" class="px-4 py-2 bg-primary-500 text-white rounded-lg font-medium hover:bg-primary-600 transition">完成</button>
        </div>
      </div>
    </div>
    
    <!-- 分组管理弹窗 -->
    <div v-if="showGroupModal" class="fixed inset-0 bg-dark-900/80 flex items-center justify-center z-50">
      <div class="bg-dark-800 rounded-xl p-6 w-full max-w-md border border-dark-600">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-semibold text-white">分组管理</h3>
          <button @click="showGroupModal = false" class="text-gray-400 hover:text-white">
            <X class="w-5 h-5" />
          </button>
        </div>
        <div class="space-y-2">
          <div v-for="group in groups" :key="group" class="flex items-center justify-between p-3 bg-dark-700 rounded-lg">
            <span class="text-white">{{ group }}</span>
            <span class="text-sm text-gray-400">{{ getGroupCount(group) }} 个探针</span>
          </div>
        </div>
        <div class="flex justify-end mt-6">
          <button @click="showGroupModal = false" class="px-4 py-2 bg-primary-500 text-white rounded-lg font-medium hover:bg-primary-600 transition">关闭</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { Download, ArrowUpCircle, Trash2, X } from 'lucide-vue-next'

const selectedType = ref('all')
const selectedGroup = ref('all')
const showInstallModal = ref(false)
const showGroupModal = ref(false)

const groups = ['华北', '华东', '华南', '中心', '边缘']

const newProbe = ref({ name: '', type: 'agent', group: '' })

const probes = ref([
  { id: 1, name: 'agent-prod-01', type: 'agent', group: '中心', version: 'v1.2.3', status: 'online', lastHeartbeat: '2分钟前' },
  { id: 2, name: 'agent-prod-02', type: 'agent', group: '中心', version: 'v1.2.3', status: 'online', lastHeartbeat: '1分钟前' },
  { id: 3, name: 'center-main', type: 'center', group: '中心', version: 'v1.2.3', status: 'online', lastHeartbeat: '30秒前' },
  { id: 4, name: 'edge-beijing', type: 'edge', group: '华北', version: 'v1.2.2', status: 'online', lastHeartbeat: '5分钟前' },
  { id: 5, name: 'edge-shanghai', type: 'edge', group: '华东', version: 'v1.2.3', status: 'offline', lastHeartbeat: '15分钟前' },
  { id: 6, name: 'edge-guangzhou', type: 'edge', group: '华南', version: 'v1.2.3', status: 'online', lastHeartbeat: '1分钟前' }
])

const filteredProbes = computed(() => {
  return probes.value.filter(probe => {
    const typeMatch = selectedType.value === 'all' || probe.type === selectedType.value
    const groupMatch = selectedGroup.value === 'all' || probe.group === selectedGroup.value
    return typeMatch && groupMatch
  })
})

function getGroupCount(group) {
  return probes.value.filter(p => p.group === group).length
}

function upgradeProbe(probe) {
  alert(`正在升级探针: ${probe.name}`)
}

function uninstallProbe(probe) {
  if (confirm(`确定要卸载探针 ${probe.name} 吗？`)) {
    probes.value = probes.value.filter(p => p.id !== probe.id)
  }
}

function installProbe() {
  showInstallModal.value = false
}
</script>

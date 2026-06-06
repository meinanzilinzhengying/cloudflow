<template>
  <div>
    <div class="flex items-center justify-between mb-6">
      <div class="flex gap-2">
        <button 
          v-for="type in configTypes" 
          :key="type.value"
          @click="activeType = type.value"
          :class="[
            'px-4 py-2 rounded-lg text-sm font-medium transition',
            activeType === type.value 
              ? 'bg-primary-500 text-white' 
              : 'bg-dark-800 text-gray-400 hover:text-white'
          ]"
        >
          {{ type.label }}
        </button>
      </div>
      <button @click="showAddModal = true" class="px-4 py-2 bg-primary-500 text-white text-sm font-medium rounded-lg hover:bg-primary-600 transition flex items-center gap-2">
        <Plus class="w-4 h-4" />
        添加配置
      </button>
    </div>
    
    <div class="bg-dark-800 rounded-xl border border-dark-600 overflow-hidden">
      <table class="w-full">
        <thead>
          <tr class="border-b border-dark-600">
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">配置项</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">当前值</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">描述</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">类型</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="config in filteredConfigs" :key="config.id" class="border-b border-dark-700 hover:bg-dark-700/50 transition">
            <td class="px-4 py-3 text-sm font-mono text-primary-400">{{ config.key }}</td>
            <td class="px-4 py-3 text-sm text-white">{{ config.value }}</td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ config.description }}</td>
            <td class="px-4 py-3">
              <span 
                class="px-2 py-1 text-xs rounded-full"
                :class="getTypeClass(config.type)"
              >
                {{ config.type }}
              </span>
            </td>
            <td class="px-4 py-3">
              <button @click="editConfig(config)" class="p-1.5 hover:bg-dark-600 rounded text-primary-400">
                <Pencil class="w-4 h-4" />
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    
    <!-- 添加/编辑配置弹窗 -->
    <div v-if="showAddModal || editingConfig" class="fixed inset-0 bg-dark-900/80 flex items-center justify-center z-50">
      <div class="bg-dark-800 rounded-xl p-6 w-full max-w-md border border-dark-600">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-semibold text-white">{{ editingConfig ? '编辑配置' : '添加配置' }}</h3>
          <button @click="closeModal" class="text-gray-400 hover:text-white">
            <X class="w-5 h-5" />
          </button>
        </div>
        <div class="space-y-4">
          <div>
            <label class="block text-sm text-gray-400 mb-1">配置键</label>
            <input v-model="formData.key" type="text" :disabled="!!editingConfig" class="w-full px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500 disabled:opacity-50" />
          </div>
          <div>
            <label class="block text-sm text-gray-400 mb-1">配置值</label>
            <input v-model="formData.value" type="text" class="w-full px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500" />
          </div>
          <div>
            <label class="block text-sm text-gray-400 mb-1">配置类型</label>
            <select v-model="formData.type" class="w-full px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500">
              <option value="threshold">阈值配置</option>
              <option value="notification">通知配置</option>
              <option value="general">常规配置</option>
            </select>
          </div>
          <div>
            <label class="block text-sm text-gray-400 mb-1">描述</label>
            <textarea v-model="formData.description" rows="2" class="w-full px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500"></textarea>
          </div>
        </div>
        <div class="flex justify-end gap-3 mt-6">
          <button @click="closeModal" class="px-4 py-2 text-gray-400 hover:text-white transition">取消</button>
          <button @click="saveConfig" class="px-4 py-2 bg-primary-500 text-white rounded-lg font-medium hover:bg-primary-600 transition">保存</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { Plus, Pencil, X } from 'lucide-vue-next'

const configTypes = [
  { label: '全部', value: 'all' },
  { label: '阈值配置', value: 'threshold' },
  { label: '通知配置', value: 'notification' },
  { label: '常规配置', value: 'general' }
]

const activeType = ref('all')
const showAddModal = ref(false)
const editingConfig = ref(null)

const formData = ref({
  key: '',
  value: '',
  type: 'threshold',
  description: ''
})

const configs = ref([
  { id: 1, key: 'cpu_usage_threshold', value: '85', type: 'threshold', description: 'CPU使用率告警阈值(%)' },
  { id: 2, key: 'memory_usage_threshold', value: '90', type: 'threshold', description: '内存使用率告警阈值(%)' },
  { id: 3, key: 'disk_usage_threshold', value: '95', type: 'threshold', description: '磁盘使用率告警阈值(%)' },
  { id: 4, key: 'webhook_url', value: 'https://hooks.slack.com/services/xxx', type: 'notification', description: '告警通知Webhook地址' },
  { id: 5, key: 'notification_interval', value: '60', type: 'notification', description: '告警通知间隔(秒)' },
  { id: 6, key: 'log_level', value: 'info', type: 'general', description: '系统日志级别' },
  { id: 7, key: 'retention_days', value: '30', type: 'general', description: '数据保留天数' }
])

const filteredConfigs = computed(() => {
  if (activeType.value === 'all') return configs.value
  return configs.value.filter(c => c.type === activeType.value)
})

function getTypeClass(type) {
  if (type === 'threshold') return 'bg-blue-500/20 text-blue-400'
  if (type === 'notification') return 'bg-purple-500/20 text-purple-400'
  return 'bg-gray-500/20 text-gray-400'
}

function editConfig(config) {
  editingConfig.value = config
  formData.value = { ...config }
}

function closeModal() {
  showAddModal.value = false
  editingConfig.value = null
  formData.value = { key: '', value: '', type: 'threshold', description: '' }
}

function saveConfig() {
  if (editingConfig.value) {
    const index = configs.value.findIndex(c => c.id === editingConfig.value.id)
    if (index !== -1) {
      configs.value[index] = { ...editingConfig.value, ...formData.value }
    }
  } else {
    configs.value.push({
      id: Date.now(),
      ...formData.value
    })
  }
  closeModal()
}
</script>

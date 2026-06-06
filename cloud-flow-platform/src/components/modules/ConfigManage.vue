<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <h2 class="text-xl font-semibold text-white">配置管理</h2>
      <button @click="showAddModal = true" class="px-4 py-2 bg-accent-500 text-dark-300 rounded-lg font-medium hover:bg-accent-400 transition-colors flex items-center gap-2">
        <Plus class="w-4 h-4" />
        添加配置
      </button>
    </div>

    <!-- 配置分类 -->
    <div class="flex gap-2">
      <button 
        v-for="type in configTypes" 
        :key="type.value"
        @click="activeType = type.value"
        :class="['px-4 py-2 rounded-lg text-sm font-medium transition-colors', activeType === type.value ? 'bg-accent-500 text-dark-300' : 'bg-dark-200 text-gray-400 hover:text-white']"
      >
        {{ type.label }}
      </button>
    </div>

    <!-- 配置列表 -->
    <div class="bg-dark-200 rounded-xl border border-dark-100 overflow-hidden">
      <table class="w-full">
        <thead class="bg-dark-300">
          <tr>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">配置项</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">当前值</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">描述</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">类型</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">操作</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-dark-100">
          <tr v-for="config in filteredConfigs" :key="config.id" class="hover:bg-dark-100/50">
            <td class="px-4 py-3 text-sm font-mono text-accent-400">{{ config.key }}</td>
            <td class="px-4 py-3 text-sm text-white">{{ config.value }}</td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ config.description }}</td>
            <td class="px-4 py-3">
              <span class="px-2 py-0.5 text-xs rounded-full" :class="getTypeClass(config.type)">
                {{ config.type }}
              </span>
            </td>
            <td class="px-4 py-3">
              <button @click="editConfig(config)" class="p-1.5 hover:bg-dark-100 rounded text-accent-500">
                <Pencil class="w-4 h-4" />
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 添加/编辑配置弹窗 -->
    <div v-if="showAddModal || editingConfig" class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div class="bg-dark-200 rounded-xl p-6 w-full max-w-md border border-dark-100">
        <h3 class="text-lg font-semibold text-white mb-4">{{ editingConfig ? '编辑配置' : '添加配置' }}</h3>
        <div class="space-y-4">
          <div>
            <label class="block text-sm text-gray-400 mb-1">配置键</label>
            <input v-model="formData.key" type="text" :disabled="!!editingConfig" class="w-full px-3 py-2 bg-dark-300 border border-dark-100 rounded-lg text-white focus:outline-none focus:border-accent-500 disabled:opacity-50" />
          </div>
          <div>
            <label class="block text-sm text-gray-400 mb-1">配置值</label>
            <input v-model="formData.value" type="text" class="w-full px-3 py-2 bg-dark-300 border border-dark-100 rounded-lg text-white focus:outline-none focus:border-accent-500" />
          </div>
          <div>
            <label class="block text-sm text-gray-400 mb-1">配置类型</label>
            <select v-model="formData.type" class="w-full px-3 py-2 bg-dark-300 border border-dark-100 rounded-lg text-white focus:outline-none focus:border-accent-500">
              <option value="threshold">阈值配置</option>
              <option value="notification">通知配置</option>
              <option value="general">常规配置</option>
            </select>
          </div>
          <div>
            <label class="block text-sm text-gray-400 mb-1">描述</label>
            <textarea v-model="formData.description" rows="2" class="w-full px-3 py-2 bg-dark-300 border border-dark-100 rounded-lg text-white focus:outline-none focus:border-accent-500"></textarea>
          </div>
        </div>
        <div class="flex justify-end gap-3 mt-6">
          <button @click="closeModal" class="px-4 py-2 text-gray-400 hover:text-white">取消</button>
          <button @click="saveConfig" class="px-4 py-2 bg-accent-500 text-dark-300 rounded-lg font-medium hover:bg-accent-400">保存</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import api from '../../api'
import { Plus, Pencil } from 'lucide-vue-next'

const configs = ref([])
const showAddModal = ref(false)
const editingConfig = ref(null)
const activeType = ref('all')

const configTypes = [
  { label: '全部', value: 'all' },
  { label: '阈值配置', value: 'threshold' },
  { label: '通知配置', value: 'notification' },
  { label: '常规配置', value: 'general' }
]

const formData = ref({
  key: '',
  value: '',
  type: 'threshold',
  description: ''
})

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

onMounted(async () => {
  configs.value = await api.getConfigs()
})
</script>

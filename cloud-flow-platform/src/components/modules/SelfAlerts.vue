<template>
  <div>
    <div class="flex items-center justify-between mb-6">
      <h3 class="font-semibold text-white">平台告警</h3>
      <button @click="showRuleModal = true" class="px-4 py-2 bg-primary-500 text-white text-sm font-medium rounded-lg hover:bg-primary-600 transition flex items-center gap-2">
        <Plus class="w-4 h-4" />
        添加规则
      </button>
    </div>
    
    <div class="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
      <div class="bg-dark-800 rounded-xl p-4 border border-dark-600">
        <div class="text-sm text-gray-400 mb-1">全部告警</div>
        <div class="text-2xl font-bold text-white">{{ alerts.length }}</div>
      </div>
      <div class="bg-dark-800 rounded-xl p-4 border border-dark-600">
        <div class="text-sm text-gray-400 mb-1">触发中</div>
        <div class="text-2xl font-bold text-red-400">{{ firingCount }}</div>
      </div>
      <div class="bg-dark-800 rounded-xl p-4 border border-dark-600">
        <div class="text-sm text-gray-400 mb-1">已解决</div>
        <div class="text-2xl font-bold text-green-400">{{ resolvedCount }}</div>
      </div>
      <div class="bg-dark-800 rounded-xl p-4 border border-dark-600">
        <div class="text-sm text-gray-400 mb-1">规则总数</div>
        <div class="text-2xl font-bold text-white">12</div>
      </div>
    </div>
    
    <div class="bg-dark-800 rounded-xl border border-dark-600 overflow-hidden">
      <div class="px-4 py-3 border-b border-dark-600 flex items-center justify-between">
        <h3 class="font-medium text-white">告警列表</h3>
        <div class="flex gap-2">
          <button 
            v-for="status in alertStatuses" 
            :key="status.value"
            @click="activeStatus = status.value"
            :class="[
              'px-3 py-1 rounded text-xs font-medium transition',
              activeStatus === status.value 
                ? 'bg-primary-500 text-white' 
                : 'bg-dark-700 text-gray-400'
            ]"
          >
            {{ status.label }}
          </button>
        </div>
      </div>
      <table class="w-full">
        <thead>
          <tr class="border-b border-dark-600">
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">告警级别</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">告警标题</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">来源</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">发生时间</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">状态</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="alert in filteredAlerts" :key="alert.id" class="border-b border-dark-700 hover:bg-dark-700/50 transition">
            <td class="px-4 py-3">
              <span 
                class="px-2 py-1 text-xs rounded-full font-medium"
                :class="getLevelClass(alert.level)"
              >
                {{ getLevelText(alert.level) }}
              </span>
            </td>
            <td class="px-4 py-3 text-sm text-white">{{ alert.title }}</td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ alert.source }}</td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ alert.time }}</td>
            <td class="px-4 py-3">
              <span 
                class="px-2 py-1 text-xs rounded-full"
                :class="alert.status === 'firing' ? 'bg-red-500/20 text-red-400' : 'bg-green-500/20 text-green-400'"
              >
                {{ alert.status === 'firing' ? '触发中' : '已解决' }}
              </span>
            </td>
            <td class="px-4 py-3">
              <div class="flex gap-2">
                <button v-if="alert.status === 'firing'" @click="resolveAlert(alert)" class="p-1.5 hover:bg-dark-600 rounded text-green-400" title="标记已解决">
                  <CheckCircle class="w-4 h-4" />
                </button>
                <button class="p-1.5 hover:bg-dark-600 rounded text-gray-400" title="详情">
                  <Eye class="w-4 h-4" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    
    <!-- 添加规则弹窗 -->
    <div v-if="showRuleModal" class="fixed inset-0 bg-dark-900/80 flex items-center justify-center z-50">
      <div class="bg-dark-800 rounded-xl p-6 w-full max-w-lg border border-dark-600">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-semibold text-white">添加告警规则</h3>
          <button @click="showRuleModal = false" class="text-gray-400 hover:text-white">
            <X class="w-5 h-5" />
          </button>
        </div>
        <div class="space-y-4">
          <div>
            <label class="block text-sm text-gray-400 mb-1">规则名称</label>
            <input v-model="ruleForm.name" type="text" class="w-full px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500" placeholder="输入规则名称" />
          </div>
          <div>
            <label class="block text-sm text-gray-400 mb-1">告警级别</label>
            <select v-model="ruleForm.level" class="w-full px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500">
              <option value="critical">严重</option>
              <option value="warning">警告</option>
              <option value="info">信息</option>
            </select>
          </div>
          <div>
            <label class="block text-sm text-gray-400 mb-1">条件表达式</label>
            <input v-model="ruleForm.condition" type="text" class="w-full px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500 font-mono" placeholder="cpu_usage > 80" />
          </div>
          <div>
            <label class="block text-sm text-gray-400 mb-1">告警间隔 (秒)</label>
            <input v-model="ruleForm.interval" type="number" class="w-full px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500" />
          </div>
        </div>
        <div class="flex justify-end gap-3 mt-6">
          <button @click="showRuleModal = false" class="px-4 py-2 text-gray-400 hover:text-white transition">取消</button>
          <button @click="saveRule" class="px-4 py-2 bg-primary-500 text-white rounded-lg font-medium hover:bg-primary-600 transition">保存</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { Plus, CheckCircle, Eye, X } from 'lucide-vue-next'

const alertStatuses = [
  { label: '全部', value: 'all' },
  { label: '触发中', value: 'firing' },
  { label: '已解决', value: 'resolved' }
]

const activeStatus = ref('all')
const showRuleModal = ref(false)

const ruleForm = ref({
  name: '',
  level: 'warning',
  condition: '',
  interval: 60
})

const alerts = ref([])

const filteredAlerts = computed(() => {
  if (activeStatus.value === 'all') return alerts.value
  return alerts.value.filter(a => a.status === activeStatus.value)
})

const firingCount = computed(() => alerts.value.filter(a => a.status === 'firing').length)
const resolvedCount = computed(() => alerts.value.filter(a => a.status === 'resolved').length)

function getLevelClass(level) {
  if (level === 'critical') return 'bg-red-500/20 text-red-400'
  if (level === 'warning') return 'bg-yellow-500/20 text-yellow-400'
  return 'bg-blue-500/20 text-blue-400'
}

function getLevelText(level) {
  if (level === 'critical') return '严重'
  if (level === 'warning') return '警告'
  return '信息'
}

function resolveAlert(alert) {
  alert.status = 'resolved'
}

function saveRule() {
  showRuleModal.value = false
}
</script>

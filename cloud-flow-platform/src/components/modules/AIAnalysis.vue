<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-white">AI 智能分析</h1>
        <p class="text-gray-400 mt-1">使用 AI 助手分析网络流量数据</p>
      </div>
    </div>

    <!-- 模型配置 -->
    <div class="bg-dark-800 rounded-xl border border-dark-700 p-6">
      <h2 class="text-lg font-semibold text-white mb-4 flex items-center gap-2">
        <Settings class="w-5 h-5 text-blue-400" />
        AI 模型配置
      </h2>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div>
          <label class="block text-sm font-medium text-gray-300 mb-2">选择模型</label>
          <select
            v-model="selectedModel"
            @change="loadModels"
            class="w-full bg-dark-900 border border-dark-700 text-white rounded-lg px-4 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            <option v-for="model in models" :key="model.name" :value="model.name">
              {{ model.name }} ({{ model.provider }})
            </option>
          </select>
        </div>
        <div>
          <label class="block text-sm font-medium text-gray-300 mb-2">API 地址</label>
          <input
            type="text"
            v-model="apiUrl"
            placeholder="http://localhost:8082"
            class="w-full bg-dark-900 border border-dark-700 text-white rounded-lg px-4 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>
      </div>
      <div class="mt-4">
        <button
          @click="testConnection"
          class="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition"
        >
          测试连接
        </button>
        <span v-if="connectionStatus" :class="connectionStatus.success ? 'text-green-400' : 'text-red-400'" class="ml-3">
          {{ connectionStatus.message }}
        </span>
      </div>
    </div>

    <!-- 分析区域 -->
    <div class="bg-dark-800 rounded-xl border border-dark-700 p-6">
      <h2 class="text-lg font-semibold text-white mb-4 flex items-center gap-2">
        <MessageSquare class="w-5 h-5 text-green-400" />
        智能问答
      </h2>

      <div class="space-y-4">
        <div class="flex gap-2">
          <input
            type="text"
            v-model="query"
            @keyup.enter="submitQuery"
            placeholder="请输入你想分析的问题，例如：'为什么最近网络延迟变高了？'"
            class="flex-1 bg-dark-900 border border-dark-700 text-white rounded-lg px-4 py-3 focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
          <button
            @click="submitQuery"
            :disabled="loading || !query.trim()"
            class="px-6 py-3 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed text-white rounded-lg transition"
          >
            {{ loading ? '分析中...' : '发送' }}
          </button>
        </div>
      </div>

      <!-- 快捷问题 -->
      <div class="mt-4">
        <span class="text-sm text-gray-400">快捷问题：</span>
        <div class="flex flex-wrap gap-2 mt-2">
          <button
            v-for="q in quickQuestions"
            :key="q"
            @click="query = q"
            class="px-3 py-1 bg-dark-700 hover:bg-dark-600 text-sm text-gray-300 rounded-lg transition"
          >
            {{ q }}
          </button>
        </div>
      </div>
    </div>

    <!-- 分析结果 -->
    <div v-if="analyses.length > 0" class="space-y-4">
      <div v-for="item in analyses" :key="item.id" class="bg-dark-800 rounded-xl border border-dark-700 overflow-hidden">
        <div class="p-4 border-b border-dark-700 flex items-center justify-between">
          <div class="flex items-center gap-3">
            <div class="w-8 h-8 bg-blue-600 rounded-full flex items-center justify-center">
              <MessageCircle class="w-4 h-4 text-white" />
            </div>
            <div>
              <span class="text-sm text-gray-400">用户提问</span>
              <span class="text-xs text-gray-500 ml-2">{{ item.created_at }}</span>
            </div>
          </div>
          <span class="text-xs text-gray-500 bg-dark-700 px-2 py-1 rounded">模型: {{ item.model }}</span>
        </div>
        <div class="p-4">
          <p class="text-gray-300">{{ item.query }}</p>
        </div>
        <div class="p-4 bg-dark-750">
          <div class="flex items-start gap-3">
            <div class="w-8 h-8 bg-green-600 rounded-full flex items-center justify-center flex-shrink-0">
              <Bot class="w-4 h-4 text-white" />
            </div>
            <div class="flex-1">
              <span class="text-sm text-gray-400">AI 回复</span>
              <div class="mt-2 text-gray-200 whitespace-pre-wrap">{{ item.result }}</div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 历史记录 -->
    <div v-if="history.length > 0" class="bg-dark-800 rounded-xl border border-dark-700 p-6">
      <div class="flex items-center justify-between mb-4">
        <h2 class="text-lg font-semibold text-white flex items-center gap-2">
          <History class="w-5 h-5 text-purple-400" />
          分析历史
        </h2>
        <button
          @click="clearHistory"
          class="text-sm text-red-400 hover:text-red-300"
        >
          清空历史
        </button>
      </div>
      <div class="space-y-2 max-h-64 overflow-y-auto">
        <div
          v-for="item in history.slice().reverse()"
          :key="item.id"
          @click="loadHistoryItem(item)"
          class="p-3 bg-dark-900 rounded-lg hover:bg-dark-750 cursor-pointer transition"
        >
          <div class="flex items-center justify-between">
            <p class="text-sm text-gray-300 truncate">{{ item.query }}</p>
            <span class="text-xs text-gray-500">{{ item.created_at }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { Settings, MessageSquare, MessageCircle, Bot, History } from 'lucide-vue-next'

const selectedModel = ref('')
const apiUrl = ref('http://localhost:8082')
const models = ref([])
const connectionStatus = ref(null)

const query = ref('')
const loading = ref(false)
const analyses = ref([])
const history = ref([])

const quickQuestions = [
  '为什么网络流量突然增加了？',
  '分析一下最近的延迟异常',
  '有什么安全问题需要注意？',
  '如何优化当前的网络性能？',
  '请帮我分析一下今天的流量数据'
]

onMounted(() => {
  loadModels()
  loadHistory()
})

async function loadModels() {
  try {
    const res = await fetch(`${apiUrl.value}/api/v1/models`)
    const data = await res.json()
    models.value = data.models || []
    if (data.default && models.value.length > 0) {
      selectedModel.value = data.default
    } else if (models.value.length > 0) {
      selectedModel.value = models.value[0].name
    }
  } catch (e) {
    console.error('Failed to load models', e)
  }
}

async function testConnection() {
  try {
    const res = await fetch(`${apiUrl.value}/health`)
    const data = await res.json()
    if (data.status === 'ok') {
      connectionStatus.value = { success: true, message: '连接成功！' }
      loadModels()
    }
  } catch (e) {
    connectionStatus.value = { success: false, message: '连接失败: ' + e.message }
  }
}

async function submitQuery() {
  if (!query.value.trim()) return

  loading.value = true
  try {
    const reqBody = {
      query: query.value,
      model: selectedModel.value
    }

    const res = await fetch(`${apiUrl.value}/api/v1/analyze`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(reqBody)
    })

    if (!res.ok) throw new Error('Request failed')

    const data = await res.json()
    const newAnalysis = {
      id: data.id,
      query: query.value,
      result: data.result,
      model: data.model,
      created_at: new Date().toLocaleString()
    }
    analyses.value.unshift(newAnalysis)
    history.value.push(newAnalysis)
    saveHistory()
    query.value = ''
  } catch (e) {
    alert('分析失败: ' + e.message)
  } finally {
    loading.value = false
  }
}

function loadHistory() {
  const saved = localStorage.getItem('ai_analysis_history')
  if (saved) {
    history.value = JSON.parse(saved)
  }
}

function saveHistory() {
  localStorage.setItem('ai_analysis_history', JSON.stringify(history.value))
}

function clearHistory() {
  if (confirm('确定清空所有历史记录吗？')) {
    history.value = []
    localStorage.removeItem('ai_analysis_history')
  }
}

function loadHistoryItem(item) {
  query.value = item.query
}
</script>

<style scoped>
.dark-750 {
  background-color: rgba(30, 41, 59, 0.5);
}
</style>

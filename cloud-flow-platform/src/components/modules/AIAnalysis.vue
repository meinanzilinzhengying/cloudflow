<template>
  <div class="ai-analysis-container space-y-6">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <div>
        <h2 class="text-2xl font-bold text-white">AI 分析配置</h2>
        <p class="text-gray-400 mt-1">配置和管理 AI 模型，进行智能分析</p>
      </div>
      <div class="flex items-center space-x-2">
        <div
          :class="[
            'w-3 h-3 rounded-full',
            backendStatus.connected ? 'bg-green-500' : 'bg-red-500'
          ]"
        ></div>
        <span class="text-sm text-gray-400">
          {{ backendStatus.connected ? '后端已连接' : '后端未连接' }}
        </span>
      </div>
    </div>

    <!-- API Configuration Section -->
    <div class="bg-dark-800 rounded-xl p-6 border border-dark-700">
      <h3 class="text-lg font-semibold text-white mb-4">API 配置</h3>
      <div class="space-y-4">
        <div>
          <label class="block text-sm font-medium text-gray-300 mb-2">
            Backend API 地址
          </label>
          <div class="flex space-x-2">
            <input
              v-model="apiConfig.url"
              type="text"
              placeholder="http://localhost:8082"
              class="flex-1 px-4 py-2 bg-dark-900 border border-dark-700 rounded-lg text-white placeholder-gray-500 focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              @change="saveApiUrl"
              @blur="saveApiUrl"
            />
            <button
              @click="testBackendConnection"
              :disabled="testingConnection"
              class="px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:bg-gray-600 text-white rounded-lg transition-colors duration-200"
            >
              <Loader2 v-if="testingConnection" class="w-4 h-4 animate-spin" />
              <span v-else>测试连接</span>
            </button>
          </div>
          <p v-if="connectionTestResult" :class="['mt-2 text-sm', connectionTestResult.success ? 'text-green-400' : 'text-red-400']">
            {{ connectionTestResult.message }}
          </p>
        </div>
      </div>
    </div>

    <!-- Model Selection Section -->
    <div class="bg-dark-800 rounded-xl p-6 border border-dark-700">
      <h3 class="text-lg font-semibold text-white mb-4">模型选择</h3>
      <div class="space-y-4">
        <div>
          <label class="block text-sm font-medium text-gray-300 mb-2">
            选择 AI 模型
          </label>
          <select
            v-model="selectedModel"
            @change="saveSelectedModel"
            class="w-full px-4 py-2 bg-dark-900 border border-dark-700 rounded-lg text-white focus:ring-2 focus:ring-blue-500 focus:border-transparent"
          >
            <option value="" disabled>{{ models.length === 0 ? '暂无可用模型，请先配置' : '请选择模型' }}</option>
            <option
              v-for="model in models"
              :key="model.name"
              :value="model.name"
            >
              {{ model.name }} ({{ getProviderLabel(model.provider) }})
              {{ model.is_default ? '- 默认' : '' }}
            </option>
          </select>
        </div>

        <div v-if="models.length === 0" class="text-center py-4">
          <p class="text-gray-400 mb-2">暂无配置的模型</p>
          <button
            @click="showAdvancedConfig = true"
            class="text-blue-400 hover:text-blue-300 text-sm"
          >
            前往高级配置添加模型 →
          </button>
        </div>
      </div>
    </div>

    <!-- Advanced Configuration (Model Management) -->
    <div class="bg-dark-800 rounded-xl p-6 border border-dark-700">
      <div
        class="flex items-center justify-between cursor-pointer"
        @click="showAdvancedConfig = !showAdvancedConfig"
      >
        <h3 class="text-lg font-semibold text-white">高级配置</h3>
        <ChevronDown
          :class="['w-5 h-5 text-gray-400 transition-transform duration-200', showAdvancedConfig ? 'transform rotate-180' : '']"
        />
      </div>

      <div v-if="showAdvancedConfig" class="mt-4 space-y-6">
        <!-- Model List Table -->
        <div>
          <div class="flex items-center justify-between mb-4">
            <h4 class="text-md font-medium text-white">已配置的模型</h4>
            <button
              @click="openAddModelDialog"
              class="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors duration-200 flex items-center space-x-2"
            >
              <Plus class="w-4 h-4" />
              <span>添加模型</span>
            </button>
          </div>

          <div class="overflow-x-auto">
            <table class="w-full text-sm text-left text-gray-300">
              <thead class="text-xs uppercase bg-dark-900 text-gray-400">
                <tr>
                  <th class="px-4 py-3">模型名称</th>
                  <th class="px-4 py-3">提供商</th>
                  <th class="px-4 py-3">API 地址</th>
                  <th class="px-4 py-3">默认</th>
                  <th class="px-4 py-3">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="model in models"
                  :key="model.name"
                  class="border-b border-dark-700 hover:bg-dark-900/50"
                >
                  <td class="px-4 py-3 font-medium text-white">{{ model.name }}</td>
                  <td class="px-4 py-3">{{ getProviderLabel(model.provider) }}</td>
                  <td class="px-4 py-3 text-xs">{{ model.api_url }}</td>
                  <td class="px-4 py-3">
                    <span v-if="model.is_default" class="text-green-400">✓</span>
                  </td>
                  <td class="px-4 py-3">
                    <div class="flex space-x-2">
                      <button
                        @click="editModel(model)"
                        class="text-blue-400 hover:text-blue-300"
                        title="编辑"
                      >
                        <Edit class="w-4 h-4" />
                      </button>
                      <button
                        @click="deleteModel(model.name)"
                        class="text-red-400 hover:text-red-300"
                        title="删除"
                      >
                        <Trash2 class="w-4 h-4" />
                      </button>
                      <button
                        @click="setAsDefault(model.name)"
                        class="text-gray-400 hover:text-yellow-300"
                        title="设为默认"
                        :disabled="model.is_default"
                      >
                        <Star class="w-4 h-4" :class="{ 'text-yellow-400': model.is_default }" />
                      </button>
                    </div>
                  </td>
                </tr>
                <tr v-if="models.length === 0">
                  <td colspan="5" class="px-4 py-8 text-center text-gray-500">
                    暂无模型配置
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- Add/Edit Model Form (Dialog) -->
        <div
          v-if="showModelDialog"
          class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
          @click.self="closeModelDialog"
        >
          <div class="bg-dark-800 rounded-xl p-6 w-full max-w-2xl border border-dark-700 max-h-[90vh] overflow-y-auto">
            <h4 class="text-lg font-semibold text-white mb-4">
              {{ editingModel ? '编辑模型' : '添加模型' }}
            </h4>

            <form @submit.prevent="saveModel" class="space-y-4">
              <!-- Model Name -->
              <div>
                <label class="block text-sm font-medium text-gray-300 mb-2">
                  模型名称 *
                </label>
                <input
                  v-model="modelForm.name"
                  type="text"
                  required
                  placeholder="例如: gpt-4o, llama3:latest"
                  class="w-full px-4 py-2 bg-dark-900 border border-dark-700 rounded-lg text-white placeholder-gray-500 focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                />
              </div>

              <!-- Provider -->
              <div>
                <label class="block text-sm font-medium text-gray-300 mb-2">
                  提供商 *
                </label>
                <select
                  v-model="modelForm.provider"
                  required
                  class="w-full px-4 py-2 bg-dark-900 border border-dark-700 rounded-lg text-white focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                  @change="onProviderChange"
                >
                  <option value="openai">OpenAI</option>
                  <option value="deepseek">DeepSeek</option>
                  <option value="qwen">Qwen (阿里云)</option>
                  <option value="ollama">Ollama (本地)</option>
                  <option value="custom">自定义</option>
                </select>
              </div>

              <!-- API URL -->
              <div>
                <label class="block text-sm font-medium text-gray-300 mb-2">
                  API 地址 *
                </label>
                <div class="flex space-x-2">
                  <input
                    v-model="modelForm.api_url"
                    type="text"
                    required
                    placeholder="https://api.openai.com/v1"
                    class="flex-1 px-4 py-2 bg-dark-900 border border-dark-700 rounded-lg text-white placeholder-gray-500 focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                  />
                  <button
                    v-if="modelForm.provider === 'ollama'"
                    @click="discoverOllamaModels"
                    :disabled="discoveringOllama"
                    type="button"
                    class="px-4 py-2 bg-dark-700 hover:bg-dark-600 text-white rounded-lg transition-colors duration-200 whitespace-nowrap"
                  >
                    <Loader2 v-if="discoveringOllama" class="w-4 h-4 animate-spin" />
                    <span v-else>发现本地模型</span>
                  </button>
                </div>
              </div>

              <!-- Ollama Models Dropdown -->
              <div v-if="modelForm.provider === 'ollama' && ollamaModels.length > 0">
                <label class="block text-sm font-medium text-gray-300 mb-2">
                  发现的 Ollama 模型
                </label>
                <select
                  @change="selectOllamaModel($event.target.value)"
                  class="w-full px-4 py-2 bg-dark-900 border border-dark-700 rounded-lg text-white focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                >
                  <option value="" disabled>选择发现的模型</option>
                  <option
                    v-for="model in ollamaModels"
                    :key="model"
                    :value="model"
                  >
                    {{ model }}
                  </option>
                </select>
              </div>

              <!-- API Key -->
              <div>
                <label class="block text-sm font-medium text-gray-300 mb-2">
                  API Key
                  <span class="text-gray-500 text-xs ml-1">(可选，Ollama 不需要)</span>
                </label>
                <input
                  v-model="modelForm.api_key"
                  type="password"
                  placeholder="sk-..."
                  class="w-full px-4 py-2 bg-dark-900 border border-dark-700 rounded-lg text-white placeholder-gray-500 focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                />
              </div>

              <!-- Max Tokens -->
              <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div>
                  <label class="block text-sm font-medium text-gray-300 mb-2">
                    Max Tokens
                  </label>
                  <input
                    v-model.number="modelForm.max_tokens"
                    type="number"
                    min="1"
                    max="200000"
                    placeholder="4096"
                    class="w-full px-4 py-2 bg-dark-900 border border-dark-700 rounded-lg text-white placeholder-gray-500 focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                  />
                </div>

                <!-- Temperature -->
                <div>
                  <label class="block text-sm font-medium text-gray-300 mb-2">
                    Temperature
                  </label>
                  <input
                    v-model.number="modelForm.temperature"
                    type="number"
                    min="0"
                    max="2"
                    step="0.1"
                    placeholder="0.7"
                    class="w-full px-4 py-2 bg-dark-900 border border-dark-700 rounded-lg text-white placeholder-gray-500 focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                  />
                </div>
              </div>

              <!-- Form Actions -->
              <div class="flex items-center justify-between pt-4 border-t border-dark-700">
                <div class="flex space-x-2">
                  <button
                    type="button"
                    @click="testModelConnection"
                    :disabled="testingModel"
                    class="px-4 py-2 bg-dark-700 hover:bg-dark-600 text-white rounded-lg transition-colors duration-200"
                  >
                    <Loader2 v-if="testingModel" class="w-4 h-4 animate-spin inline mr-2" />
                    <span>测试连接</span>
                  </button>
                </div>

                <div class="flex space-x-2">
                  <button
                    type="button"
                    @click="closeModelDialog"
                    class="px-4 py-2 bg-dark-700 hover:bg-dark-600 text-white rounded-lg transition-colors duration-200"
                  >
                    取消
                  </button>
                  <button
                    type="submit"
                    :disabled="savingModel"
                    class="px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:bg-gray-600 text-white rounded-lg transition-colors duration-200"
                  >
                    <Loader2 v-if="savingModel" class="w-4 h-4 animate-spin inline mr-2" />
                    <span>保存</span>
                  </button>
                </div>
              </div>

              <!-- Test Result -->
              <p v-if="modelTestResult" :class="['text-sm mt-2', modelTestResult.success ? 'text-green-400' : 'text-red-400']">
                {{ modelTestResult.message }}
              </p>
            </form>
          </div>
        </div>
      </div>
    </div>

    <!-- Chat/Analysis Section -->
    <div class="bg-dark-800 rounded-xl p-6 border border-dark-700">
      <h3 class="text-lg font-semibold text-white mb-4">AI 分析</h3>

      <!-- Current Model Display -->
      <div class="mb-4 p-3 bg-dark-900 rounded-lg">
        <div class="flex items-center justify-between">
          <div class="flex items-center space-x-2">
            <Cpu class="w-4 h-4 text-blue-400" />
            <span class="text-sm text-gray-300">当前模型:</span>
            <span class="text-sm text-white font-medium">{{ selectedModel || '未选择' }}</span>
          </div>
          <button
            @click="clearHistory"
            class="text-xs text-gray-400 hover:text-red-400 transition-colors duration-200"
          >
            清空历史
          </button>
        </div>
      </div>

      <!-- Quick Questions -->
      <div class="mb-4">
        <div class="flex flex-wrap gap-2">
          <button
            v-for="question in quickQuestions"
            :key="question"
            @click="setQuery(question)"
            class="px-3 py-1.5 bg-dark-900 hover:bg-dark-700 text-gray-300 hover:text-white rounded-lg text-sm transition-colors duration-200"
          >
            {{ question }}
          </button>
        </div>
      </div>

      <!-- Query Input -->
      <div class="space-y-4">
        <textarea
          v-model="userQuery"
          placeholder="输入您的问题，让 AI 帮您分析..."
          rows="4"
          class="w-full px-4 py-3 bg-dark-900 border border-dark-700 rounded-lg text-white placeholder-gray-500 focus:ring-2 focus:ring-blue-500 focus:border-transparent resize-none"
        ></textarea>

        <div class="flex items-center justify-between">
          <div class="flex items-center space-x-2">
            <button
              @click="submitQuery"
              :disabled="!userQuery.trim() || !selectedModel || analyzing"
              class="px-6 py-2.5 bg-blue-600 hover:bg-blue-700 disabled:bg-gray-600 disabled:cursor-not-allowed text-white rounded-lg transition-colors duration-200 flex items-center space-x-2"
            >
              <Loader2 v-if="analyzing" class="w-4 h-4 animate-spin" />
              <Send v-else class="w-4 h-4" />
              <span>{{ analyzing ? '分析中...' : '提交分析' }}</span>
            </button>
          </div>

          <span class="text-xs text-gray-500">
            {{ userQuery.length }} 字符
          </span>
        </div>
      </div>

      <!-- Analysis Results -->
      <div v-if="analysisHistory.length > 0" class="mt-6 space-y-4">
        <h4 class="text-md font-medium text-white">分析历史</h4>

        <div
          v-for="(item, index) in analysisHistory"
          :key="index"
          class="p-4 bg-dark-900 rounded-lg border border-dark-700"
        >
          <div class="mb-2">
            <p class="text-sm text-gray-400">问题:</p>
            <p class="text-white">{{ item.query }}</p>
          </div>
          <div class="mb-2">
            <p class="text-sm text-gray-400">模型:</p>
            <p class="text-sm text-blue-400">{{ item.model }}</p>
          </div>
          <div>
            <p class="text-sm text-gray-400">回答:</p>
            <div class="text-gray-300 whitespace-pre-wrap">{{ item.response }}</div>
          </div>
          <div class="mt-2 text-xs text-gray-500">
            {{ formatTime(item.timestamp) }}
          </div>
        </div>
      </div>

      <!-- Error Display -->
      <div v-if="errorMessage" class="mt-4 p-4 bg-red-900/20 border border-red-700 rounded-lg">
        <p class="text-red-400">{{ errorMessage }}</p>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, watch } from 'vue'
import {
  Loader2,
  ChevronDown,
  Plus,
  Edit,
  Trash2,
  Star,
  Cpu,
  Send
} from 'lucide-vue-next'

// ========== Constants ==========
const STORAGE_KEYS = {
  apiUrl: 'cf_ai_api_url',
  selectedModel: 'cf_ai_selected_model',
  history: 'ai_analysis_history'
}

const PROVIDER_URLS = {
  openai: 'https://api.openai.com/v1',
  deepseek: 'https://api.deepseek.com/v1',
  qwen: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
  ollama: 'http://localhost:11434'
}

const PROVIDER_LABELS = {
  openai: 'OpenAI',
  deepseek: 'DeepSeek',
  qwen: 'Qwen (阿里云)',
  ollama: 'Ollama',
  custom: '自定义'
}

const QUICK_QUESTIONS = [
  '分析当前流量趋势',
  '识别异常流量模式',
  '预测未来流量变化',
  '优化建议',
  '安全威胁分析'
]

// ========== Reactive State ==========
const apiConfig = reactive({
  url: 'http://localhost:8082'
})

const backendStatus = reactive({
  connected: false,
  checking: false
})

const models = ref([])
const selectedModel = ref('')
const showAdvancedConfig = ref(false)
const showModelDialog = ref(false)
const editingModel = ref(null)
const discoveringOllama = ref(false)
const ollamaModels = ref([])
const testingConnection = ref(false)
const testingModel = ref(false)
const savingModel = ref(false)
const analyzing = ref(false)
const connectionTestResult = ref(null)
const modelTestResult = ref(null)
const errorMessage = ref('')
const userQuery = ref('')
const analysisHistory = ref([])

const modelForm = reactive({
  name: '',
  provider: 'openai',
  api_url: 'https://api.openai.com/v1',
  api_key: '',
  max_tokens: 4096,
  temperature: 0.7,
  is_default: false
})

const quickQuestions = QUICK_QUESTIONS

// ========== Lifecycle ==========
onMounted(async () => {
  loadFromLocalStorage()
  await checkBackendConnection()
  await loadModels()
  loadHistory()
})

// ========== Watchers ==========
watch(() => modelForm.provider, (newProvider) => {
  onProviderChange(newProvider)
})

// ========== Local Storage Functions ==========
function loadFromLocalStorage() {
  try {
    const savedUrl = localStorage.getItem(STORAGE_KEYS.apiUrl)
    if (savedUrl) {
      apiConfig.url = savedUrl
    }

    const savedModel = localStorage.getItem(STORAGE_KEYS.selectedModel)
    if (savedModel) {
      selectedModel.value = savedModel
    }
  } catch (error) {
    console.error('Failed to load from localStorage:', error)
  }
}

function saveApiUrl() {
  try {
    localStorage.setItem(STORAGE_KEYS.apiUrl, apiConfig.url)
    showMessage('API 地址已保存', 'success')
  } catch (error) {
    console.error('Failed to save API URL:', error)
    showError('保存 API 地址失败')
  }
}

function saveSelectedModel() {
  try {
    localStorage.setItem(STORAGE_KEYS.selectedModel, selectedModel.value)
  } catch (error) {
    console.error('Failed to save selected model:', error)
  }
}

function loadHistory() {
  try {
    const saved = localStorage.getItem(STORAGE_KEYS.history)
    if (saved) {
      analysisHistory.value = JSON.parse(saved)
    }
  } catch (error) {
    console.error('Failed to load history:', error)
  }
}

function saveHistory() {
  try {
    localStorage.setItem(STORAGE_KEYS.history, JSON.stringify(analysisHistory.value))
  } catch (error) {
    console.error('Failed to save history:', error)
  }
}

// ========== API Functions ==========
async function checkBackendConnection() {
  backendStatus.checking = true
  connectionTestResult.value = null

  try {
    const response = await fetch(`${apiConfig.url}/health`, {
      method: 'GET',
      headers: {
        'Accept': 'application/json'
      }
    })

    if (response.ok) {
      const data = await response.json()
      backendStatus.connected = data.status === 'ok'
      connectionTestResult.value = {
        success: true,
        message: '✓ 后端连接成功'
      }
    } else {
      backendStatus.connected = false
      connectionTestResult.value = {
        success: false,
        message: `✗ 连接失败: HTTP ${response.status}`
      }
    }
  } catch (error) {
    backendStatus.connected = false
    connectionTestResult.value = {
      success: false,
      message: `✗ 无法连接到后端: ${error.message}`
    }
  } finally {
    backendStatus.checking = false
  }
}

async function testBackendConnection() {
  testingConnection.value = true
  await checkBackendConnection()
  testingConnection.value = false
}

async function loadModels() {
  try {
    const response = await fetch(`${apiConfig.url}/api/v1/config/models`, {
      method: 'GET',
      headers: {
        'Accept': 'application/json'
      }
    })

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`)
    }

    const data = await response.json()

    if (data && data.models) {
      models.value = data.models

      // Set default model
      if (data.default) {
        const defaultExists = models.value.find(m => m.name === data.default)
        if (defaultExists && !selectedModel.value) {
          selectedModel.value = data.default
          saveSelectedModel()
        }
      }

      // Validate current selection
      if (selectedModel.value) {
        const exists = models.value.find(m => m.name === selectedModel.value)
        if (!exists) {
          selectedModel.value = ''
          saveSelectedModel()
        }
      }
    }
  } catch (error) {
    console.error('Failed to load models:', error)
    showError(`加载模型列表失败: ${error.message}`)
  }
}

async function saveModelsConfig() {
  try {
    const payload = {
      models: models.value,
      default: models.value.find(m => m.is_default)?.name || models.value[0]?.name
    }

    const response = await fetch(`${apiConfig.url}/api/v1/config/models`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Accept': 'application/json'
      },
      body: JSON.stringify(payload)
    })

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`)
    }

    const data = await response.json()
    return data
  } catch (error) {
    console.error('Failed to save models config:', error)
    throw error
  }
}

async function testModelConnection() {
  testingModel.value = true
  modelTestResult.value = null

  try {
    const payload = {
      provider: modelForm.provider,
      api_url: modelForm.api_url,
      model: modelForm.name
    }

    if (modelForm.api_key && modelForm.provider !== 'ollama') {
      payload.api_key = modelForm.api_key
    }

    const response = await fetch(`${apiConfig.url}/api/v1/test`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Accept': 'application/json'
      },
      body: JSON.stringify(payload)
    })

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`)
    }

    const data = await response.json()

    if (data.success) {
      modelTestResult.value = {
        success: true,
        message: `✓ ${data.result || '连接成功'}`
      }
    } else {
      modelTestResult.value = {
        success: false,
        message: `✗ ${data.error || '连接失败'}`
      }
    }
  } catch (error) {
    modelTestResult.value = {
      success: false,
      message: `✗ 测试失败: ${error.message}`
    }
  } finally {
    testingModel.value = false
  }
}

async function discoverOllamaModels() {
  discoveringOllama.value = true
  ollamaModels.value = []

  try {
    const url = `${apiConfig.url}/api/v1/ollama/models?url=${encodeURIComponent(modelForm.api_url)}`
    const response = await fetch(url, {
      method: 'GET',
      headers: {
        'Accept': 'application/json'
      }
    })

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`)
    }

    const data = await response.json()

    if (data.success && data.models) {
      ollamaModels.value = data.models
      showMessage(`发现 ${data.models.length} 个本地模型`, 'success')
    } else {
      throw new Error(data.error || '未发现模型')
    }
  } catch (error) {
    showError(`发现 Ollama 模型失败: ${error.message}`)
  } finally {
    discoveringOllama.value = false
  }
}

// ========== Model Management Functions ==========
function openAddModelDialog() {
  editingModel.value = null
  resetModelForm()
  showModelDialog.value = true
}

function editModel(model) {
  editingModel.value = model
  Object.assign(modelForm, {
    name: model.name,
    provider: model.provider,
    api_url: model.api_url,
    api_key: model.api_key || '',
    max_tokens: model.max_tokens || 4096,
    temperature: model.temperature || 0.7,
    is_default: model.is_default || false
  })
  showModelDialog.value = true
}

function closeModelDialog() {
  showModelDialog.value = false
  editingModel.value = null
  resetModelForm()
  modelTestResult.value = null
}

function resetModelForm() {
  Object.assign(modelForm, {
    name: '',
    provider: 'openai',
    api_url: PROVIDER_URLS.openai,
    api_key: '',
    max_tokens: 4096,
    temperature: 0.7,
    is_default: false
  })
  ollamaModels.value = []
}

function onProviderChange(newProvider) {
  if (newProvider !== 'custom') {
    modelForm.api_url = PROVIDER_URLS[newProvider] || ''
  }
}

function selectOllamaModel(modelName) {
  if (modelName) {
    modelForm.name = modelName
  }
}

async function saveModel() {
  savingModel.value = true

  try {
    // Validate
    if (!modelForm.name.trim()) {
      throw new Error('模型名称不能为空')
    }

    if (!modelForm.api_url.trim()) {
      throw new Error('API 地址不能为空')
    }

    // Check for duplicates (except when editing)
    const duplicate = models.value.find(
      m => m.name === modelForm.name && (!editingModel.value || editingModel.value.name !== modelForm.name)
    )
    if (duplicate) {
      throw new Error('模型名称已存在')
    }

    // Build model object
    const modelObj = {
      name: modelForm.name.trim(),
      provider: modelForm.provider,
      api_url: modelForm.api_url.trim(),
      max_tokens: modelForm.max_tokens,
      temperature: modelForm.temperature,
      is_default: modelForm.is_default
    }

    if (modelForm.api_key && modelForm.provider !== 'ollama') {
      modelObj.api_key = modelForm.api_key
    }

    // Update or add
    if (editingModel.value) {
      const index = models.value.findIndex(m => m.name === editingModel.value.name)
      if (index !== -1) {
        models.value[index] = modelObj
      }
    } else {
      models.value.push(modelObj)
    }

    // Handle default
    if (modelForm.is_default) {
      models.value.forEach(m => {
        m.is_default = (m.name === modelObj.name)
      })
    }

    // Save to backend
    await saveModelsConfig()

    showMessage('模型配置已保存', 'success')
    closeModelDialog()
    await loadModels() // Reload to sync with backend
  } catch (error) {
    showError(error.message)
  } finally {
    savingModel.value = false
  }
}

async function deleteModel(modelName) {
  if (!confirm(`确定要删除模型 "${modelName}" 吗？`)) {
    return
  }

  try {
    const index = models.value.findIndex(m => m.name === modelName)
    if (index !== -1) {
      models.value.splice(index, 1)

      // If deleted model was selected
      if (selectedModel.value === modelName) {
        selectedModel.value = models.value[0]?.name || ''
        saveSelectedModel()
      }

      // Save to backend
      await saveModelsConfig()

      showMessage('模型已删除', 'success')
    }
  } catch (error) {
    showError(`删除模型失败: ${error.message}`)
  }
}

async function setAsDefault(modelName) {
  try {
    models.value.forEach(m => {
      m.is_default = (m.name === modelName)
    })

    await saveModelsConfig()
    showMessage(`"${modelName}" 已设为默认模型`, 'success')
  } catch (error) {
    showError(`设置默认模型失败: ${error.message}`)
  }
}

// ========== Analysis Functions ==========
function setQuery(question) {
  userQuery.value = question
}

async function submitQuery() {
  if (!userQuery.value.trim() || !selectedModel.value || analyzing.value) {
    return
  }

  analyzing.value = true
  errorMessage.value = ''

  try {
    // Get current model config
    const modelConfig = models.value.find(m => m.name === selectedModel.value)

    if (!modelConfig) {
      throw new Error('未找到选中的模型配置')
    }

    // Call backend analysis API
    const payload = {
      model: selectedModel.value,
      provider: modelConfig.provider,
      api_url: modelConfig.api_url,
      query: userQuery.value
    }

    if (modelConfig.api_key && modelConfig.provider !== 'ollama') {
      payload.api_key = modelConfig.api_key
    }

    const response = await fetch(`${apiConfig.url}/api/v1/analyze`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Accept': 'application/json'
      },
      body: JSON.stringify(payload)
    })

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`)
    }

    const data = await response.json()

    // Add to history
    analysisHistory.value.unshift({
      query: userQuery.value,
      model: selectedModel.value,
      response: data.result || data.response || '分析完成',
      timestamp: Date.now()
    })

    // Limit history size
    if (analysisHistory.value.length > 20) {
      analysisHistory.value = analysisHistory.value.slice(0, 20)
    }

    saveHistory()
    userQuery.value = ''
  } catch (error) {
    showError(`分析失败: ${error.message}`)
  } finally {
    analyzing.value = false
  }
}

function clearHistory() {
  if (!confirm('确定要清空分析历史吗？')) {
    return
  }

  analysisHistory.value = []
  saveHistory()
}

// ========== Helper Functions ==========
function getProviderLabel(provider) {
  return PROVIDER_LABELS[provider] || provider
}

function formatTime(timestamp) {
  const date = new Date(timestamp)
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}

function showMessage(message, type = 'success') {
  if (type === 'success') {
    connectionTestResult.value = { success: true, message }
    setTimeout(() => {
      connectionTestResult.value = null
    }, 3000)
  } else {
    showError(message)
  }
}

function showError(message) {
  errorMessage.value = message
  setTimeout(() => {
    errorMessage.value = ''
  }, 5000)
}
</script>

<style scoped>
.ai-analysis-container {
  max-width: 1200px;
  margin: 0 auto;
}

/* Custom scrollbar for dialog */
.max-h-\\[90vh\\] {
  scrollbar-width: thin;
  scrollbar-color: #4b5563 transparent;
}

.max-h-\\[90vh\\]::-webkit-scrollbar {
  width: 6px;
}

.max-h-\\[90vh\\]::-webkit-scrollbar-track {
  background: transparent;
}

.max-h-\\[90vh\\]::-webkit-scrollbar-thumb {
  background-color: #4b5563;
  border-radius: 3px;
}

/* Smooth transitions */
.transition-colors {
  transition-property: background-color, border-color, color, fill, stroke;
  transition-timing-function: cubic-bezier(0.4, 0, 0.2, 1);
  transition-duration: 200ms;
}

/* Loading spinner animation */
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.animate-spin {
  animation: spin 1s linear infinite;
}
</style>

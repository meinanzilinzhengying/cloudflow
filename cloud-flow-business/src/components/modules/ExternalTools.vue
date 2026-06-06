<template>
  <div class="external-tools-container">
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
      <div 
        v-for="tool in availableTools" 
        :key="tool.id"
        class="bg-dark-800 rounded-xl p-6 border border-dark-600 hover:border-primary-500/30 transition-all cursor-pointer"
        @click="openTool(tool)"
      >
        <div class="flex items-start justify-between mb-4">
          <div 
            class="w-12 h-12 rounded-lg flex items-center justify-center"
            :class="tool.bgClass"
          >
            <component :is="tool.icon" class="w-6 h-6" :class="tool.iconColor" />
          </div>
          <span 
            v-if="tool.status === 'online'" 
            class="px-2 py-1 text-xs rounded-full bg-green-500/20 text-green-400"
          >
            在线
          </span>
          <span 
            v-else 
            class="px-2 py-1 text-xs rounded-full bg-red-500/20 text-red-400"
          >
            离线
          </span>
        </div>
        <h3 class="text-lg font-semibold text-white mb-2">{{ tool.name }}</h3>
        <p class="text-gray-400 text-sm mb-4">{{ tool.description }}</p>
        <div class="flex items-center gap-2">
          <button class="flex-1 px-3 py-2 bg-primary-500/20 text-primary-400 text-sm font-medium rounded-lg hover:bg-primary-500/30 transition">
            打开
          </button>
          <button 
            v-if="tool.embedable"
            @click.stop="embedTool(tool)"
            class="px-3 py-2 bg-dark-700 text-gray-400 text-sm font-medium rounded-lg hover:bg-dark-600 transition"
          >
            嵌入
          </button>
        </div>
      </div>
    </div>
    
    <!-- 嵌入 Modal -->
    <div v-if="showEmbedModal" class="fixed inset-0 bg-dark-900/80 flex items-center justify-center z-50">
      <div class="bg-dark-800 rounded-xl p-6 w-full max-w-4xl border border-dark-600">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-semibold text-white">嵌入 {{ selectedTool?.name }}</h3>
          <button @click="showEmbedModal = false" class="text-gray-400 hover:text-white">
            <X class="w-5 h-5" />
          </button>
        </div>
        <div class="mb-4">
          <div class="flex gap-2">
            <button 
              v-for="mode in embedModes" 
              :key="mode.value"
              @click="embedMode = mode.value"
              :class="[
                'px-4 py-2 rounded-lg text-sm font-medium transition',
                embedMode === mode.value 
                  ? 'bg-primary-500 text-white' 
                  : 'bg-dark-700 text-gray-400 hover:text-white'
              ]"
            >
              {{ mode.label }}
            </button>
          </div>
        </div>
        <div class="iframe-container rounded-lg overflow-hidden border border-dark-600">
          <iframe 
            v-if="embedMode === 'iframe'"
            :src="selectedTool?.url" 
            :title="selectedTool?.name"
            frameborder="0"
            class="w-full h-[500px]"
          ></iframe>
          <div v-else class="p-8 text-center">
            <p class="text-gray-400 mb-4">在新窗口中打开</p>
            <a 
              :href="selectedTool?.url" 
              target="_blank"
              class="px-6 py-3 bg-primary-500 text-white font-medium rounded-lg hover:bg-primary-600 transition inline-flex items-center gap-2"
            >
              <ExternalLink class="w-4 h-4" />
              访问 {{ selectedTool?.name }}
            </a>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { 
  Search, 
  Database, 
  Activity, 
  LayoutDashboard,
  AlertTriangle,
  ExternalLink,
  X 
} from 'lucide-vue-next'

const showEmbedModal = ref(false)
const selectedTool = ref(null)
const embedMode = ref('iframe')

const embedModes = [
  { label: '内嵌页面', value: 'iframe' },
  { label: '新窗口', value: 'window' }
]

const externalTools = ref([
  {
    id: 'jaeger',
    name: 'Jaeger',
    url: 'http://localhost:16686',
    icon: Search,
    bgClass: 'bg-purple-500/20',
    iconColor: 'text-purple-400',
    description: '分布式追踪系统，用于分析请求链路和性能监控',
    embedable: true,
    status: 'online'
  },
  {
    id: 'clickhouse',
    name: 'ClickHouse',
    url: 'http://localhost:8123',
    icon: Database,
    bgClass: 'bg-yellow-500/20',
    iconColor: 'text-yellow-400',
    description: '列式数据存储，用于高效存储和查询流量数据',
    embedable: true,
    status: 'online'
  }
])

const availableTools = computed(() => {
  return externalTools.value
})

function openTool(tool) {
  window.open(tool.url, '_blank')
}

function embedTool(tool) {
  selectedTool.value = tool
  showEmbedModal.value = true
}
</script>

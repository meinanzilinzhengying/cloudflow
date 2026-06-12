<template>
  <div class="external-tools-container">
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
      <div
        v-for="tool in externalTools"
        :key="tool.id"
        class="bg-dark-800 rounded-xl p-6 border border-dark-600 hover:border-primary-500/30 transition-all"
      >
        <div class="flex items-start justify-between mb-4">
          <div
            class="w-12 h-12 rounded-lg flex items-center justify-center"
            :class="tool.bgClass"
          >
            <component :is="tool.icon" class="w-6 h-6" :class="tool.iconColor" />
          </div>
          <span
            :class="[
              'px-2 py-1 text-xs rounded-full font-medium',
              tool.status === 'online' ? 'bg-green-500/20 text-green-400' :
              tool.status === 'checking' ? 'bg-yellow-500/20 text-yellow-400 animate-pulse' :
              'bg-red-500/20 text-red-400'
            ]"
          >
            {{ tool.status === 'online' ? '在线' : tool.status === 'checking' ? '检测中' : '离线' }}
          </span>
        </div>
        <h3 class="text-lg font-semibold text-white mb-2">{{ tool.name }}</h3>
        <p class="text-gray-400 text-sm mb-4 leading-relaxed">{{ tool.description }}</p>
        <div class="flex items-center gap-2">
          <button
            @click="openTool(tool)"
            class="flex-1 px-3 py-2 bg-primary-500/20 text-primary-400 text-sm font-medium rounded-lg hover:bg-primary-500/30 transition-colors duration-200"
          >
            打开
          </button>
          <button
            v-if="tool.embedable"
            @click="embedTool(tool)"
            class="px-3 py-2 bg-dark-700 text-gray-400 text-sm font-medium rounded-lg hover:bg-dark-600 hover:text-white transition-colors duration-200"
          >
            嵌入
          </button>
        </div>
      </div>
    </div>

    <!-- 嵌入 Modal - 使用 nginx 反代路径，无需裸端口 -->
    <div v-if="showEmbedModal" class="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50" @click.self="showEmbedModal = false">
      <div class="bg-dark-800 rounded-xl border border-dark-600 w-full max-w-5xl mx-4 max-h-[90vh] flex flex-col">
        <!-- Modal Header -->
        <div class="flex items-center justify-between p-5 border-b border-dark-700">
          <div class="flex items-center gap-3">
            <component :is="selectedTool?.icon" class="w-5 h-5 text-primary-400" />
            <h3 class="text-lg font-semibold text-white">嵌入 {{ selectedTool?.name }}</h3>
          </div>
          <div class="flex items-center gap-3">
            <!-- 连接失败时显示提示 -->
            <span v-if="embedError" class="text-xs text-red-400 bg-red-500/10 px-3 py-1 rounded-full">
              {{ embedError }}
            </span>
            <button @click="closeEmbed" class="text-gray-400 hover:text-white transition-colors p-1">
              <X class="w-5 h-5" />
            </button>
          </div>
        </div>

        <!-- Toolbar -->
        <div class="flex items-center justify-between px-5 py-3 bg-dark-900/50 border-b border-dark-700">
          <div class="flex items-center gap-2 text-sm">
            <Globe class="w-4 h-4 text-gray-500" />
            <span class="text-gray-400 font-mono text-xs">{{ embedUrl }}</span>
          </div>
          <button
            @click="openTool(selectedTool)"
            class="px-3 py-1.5 text-xs bg-blue-500/20 text-blue-400 rounded-lg hover:bg-blue-500/30 transition-colors"
          >
            新窗口打开
          </button>
        </div>

        <!-- Modal Body - Iframe (使用反代路径) -->
        <div class="flex-1 p-4 min-h-[500px] relative bg-white rounded-b-xl overflow-hidden">
          <div v-if="iframeLoading" class="absolute inset-0 flex flex-col items-center justify-center z-10 bg-dark-900 rounded-b-xl">
            <Loader2 class="w-8 h-8 text-primary-400 animate-spin mb-3" />
            <p class="text-gray-400 text-sm">正在加载 {{ selectedTool?.name }}...</p>
          </div>
          <iframe
            ref="embedIframe"
            :src="embedUrl"
            :title="'嵌入 ' + selectedTool?.name"
            frameborder="0"
            class="w-full h-full rounded-lg bg-white"
            @load="onIframeLoad"
            @error="onIframeError"
          ></iframe>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import {
  Activity,
  LayoutDashboard,
  AlertTriangle,
  Search,
  Globe,
  X,
  Loader2
} from 'lucide-vue-next'

const showEmbedModal = ref(false)
const selectedTool = ref(null)
const iframeLoading = ref(false)
const embedError = ref('')
const embedIframe = ref(null)

// 使用相对路径（nginx 反向代理），不依赖裸端口
// 所有请求都走 3003 端口，避免跨端口连接被拒问题
const getProxyPath = (id) => {
  const paths = {
    grafana: '/proxy/grafana/',
    prometheus: '/proxy/prometheus/',
    alertmanager: '/proxy/alertmanager/',
    jaeger: '/proxy/jaeger/'
  }
  return paths[id] || null
}

// 工具列表
const externalTools = ref([
  {
    id: 'grafana',
    name: 'Grafana',
    url: '',
    icon: LayoutDashboard,
    bgClass: 'bg-orange-500/20',
    iconColor: 'text-orange-400',
    description: '可视化监控仪表盘，提供丰富的图表和面板',
    embedable: true,
    status: 'unknown'
  },
  {
    id: 'prometheus',
    name: 'Prometheus',
    url: '',
    icon: Activity,
    bgClass: 'bg-red-500/20',
    iconColor: 'text-red-400',
    description: '指标收集和查询系统，用于存储和分析监控数据',
    embedable: true,
    status: 'unknown'
  },
  {
    id: 'alertmanager',
    name: 'Alertmanager',
    url: '',
    icon: AlertTriangle,
    bgClass: 'bg-blue-500/20',
    iconColor: 'text-blue-400',
    description: '告警通知管理系统，处理告警路由和聚合',
    embedable: true,
    status: 'unknown'
  },
  {
    id: 'jaeger',
    name: 'Jaeger',
    url: '',
    icon: Search,
    bgClass: 'bg-purple-500/20',
    iconColor: 'text-purple-400',
    description: '分布式追踪系统，用于分析请求链路',
    embedable: true,
    status: 'unknown'
  }
])

// 嵌入 URL（反代路径）
const embedUrl = computed(() => {
  if (!selectedTool.value) return ''
  const path = getProxyPath(selectedTool.value.id)
  return path || ''
})

// 页面加载时检测服务状态（通过反代路径）
onMounted(() => {
  checkAllServices()
})

async function checkAllServices() {
  for (const tool of externalTools.value) {
    tool.status = 'checking'
    const proxyPath = getProxyPath(tool.id)
    if (!proxyPath) {
      tool.status = 'offline'
      continue
    }
    try {
      // GET + redirect: follow — 让浏览器跟随反代的重定向
      // nginx 已将 /login → /proxy/grafana/login，重定向仍走 3003 同源
      const controller = new AbortController()
      const timeoutId = setTimeout(() => controller.abort(), 5000)
      const res = await fetch(proxyPath, {
        method: 'GET',
        signal: controller.signal,
        redirect: 'follow'
      })
      clearTimeout(timeoutId)
      // 任何可访问的响应都算在线（200=正常页, 401=需要登录）
      tool.status = [200, 401, 403].includes(res.status) ? 'online' : 'offline'
    } catch (e) {
      // 网络错误（超时、DNS 失败等）→ 离线
      tool.status = 'offline'
    }
  }
}

function openTool(tool) {
  // 新窗口打开也走反代路径
  const proxyPath = getProxyPath(tool.id)
  if (proxyPath) {
    window.open(proxyPath + '', '_blank')
  } else if (tool.status === 'offline') {
    alert(`${tool.name} 当前离线，请检查服务状态`)
  }
}

function embedTool(tool) {
  if (!getProxyPath(tool.id)) {
    alert(`${tool.name} 暂不支持嵌入访问`)
    return
  }
  selectedTool.value = tool
  iframeLoading.value = true
  embedError.value = ''
  showEmbedModal.value = true
}

function onIframeLoad() {
  iframeLoading.value = false
  embedError.value = ''
}

function onIframeError() {
  iframeLoading.value = false
  embedError.value = '加载失败，服务可能未启动或网络不通'
}

function closeEmbed() {
  showEmbedModal.value = false
  embedError.value = ''
  // 延迟清空 src 以停止加载
  setTimeout(() => {
    selectedTool.value = null
  }, 300)
}
</script>

<style scoped>
.external-tools-container {
  padding: 0;
}
</style>

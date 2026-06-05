<template>
  <div class="external-tools-container">
    <div class="tool-header">
      <h2>{{ toolName }}</h2>
      <div class="tool-actions">
        <a :href="toolUrl" target="_blank" class="btn btn-outline">
          <ExternalLink class="w-4 h-4" />
          在新标签页打开
        </a>
        <button @click="refresh" class="btn btn-primary">
          <RefreshCw :class="{ 'animate-spin': loading }" class="w-4 h-4" />
          刷新
        </button>
      </div>
    </div>

    <div class="iframe-container" v-if="!loading">
      <iframe 
        :src="toolUrl" 
        :title="toolName"
        @load="onIframeLoad"
        frameborder="0"
        referrerpolicy="strict-origin-when-cross-origin"
      ></iframe>
    </div>

    <div v-else class="loading-container">
      <div class="spinner"></div>
      <p>正在加载 {{ toolName }}...</p>
    </div>

    <div v-if="!embedable" class="fallback-container">
      <div class="fallback-content">
        <div class="fallback-icon">
          <component :is="toolIcon" class="w-16 h-16" />
        </div>
        <h3>{{ toolName }}</h3>
        <p class="text-gray-400">{{ description }}</p>
        <a :href="toolUrl" target="_blank" class="btn btn-primary mt-4">
          <ExternalLink class="w-4 h-4" />
          访问工具
        </a>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { 
  Activity, 
  LayoutDashboard, 
  Search, 
  Database, 
  AlertTriangle, 
  RefreshCw, 
  ExternalLink 
} from 'lucide-vue-next'

const props = defineProps({
  tool: { 
    type: String, 
    required: true 
  }
})

const loading = ref(true)

const toolConfig = {
  grafana: {
    name: 'Grafana',
    url: 'http://localhost:3001',
    icon: LayoutDashboard,
    description: '可视化监控仪表盘，提供丰富的图表和面板',
    embedable: true
  },
  prometheus: {
    name: 'Prometheus',
    url: 'http://localhost:9091',
    icon: Activity,
    description: '指标收集和查询系统，用于存储和分析监控数据',
    embedable: true
  },
  jaeger: {
    name: 'Jaeger',
    url: 'http://localhost:16686',
    icon: Search,
    description: '分布式追踪系统，用于分析请求链路',
    embedable: true
  },
  clickhouse: {
    name: 'ClickHouse',
    url: 'http://localhost:8123',
    icon: Database,
    description: '列式数据存储，用于高效存储和查询流量数据',
    embedable: true
  },
  alertmanager: {
    name: 'Alertmanager',
    url: 'http://localhost:9093',
    icon: AlertTriangle,
    description: '告警通知管理系统，处理告警路由和聚合',
    embedable: true
  }
}

const config = computed(() => toolConfig[props.tool])
const toolName = computed(() => config.value?.name || '外部工具')
const toolUrl = computed(() => config.value?.url || '')
const toolIcon = computed(() => config.value?.icon || Activity)
const description = computed(() => config.value?.description || '')
const embedable = computed(() => config.value?.embedable !== false)

const refresh = () => {
  loading.value = true
  setTimeout(() => {
    loading.value = false
  }, 300)
}

const onIframeLoad = () => {
  loading.value = false
}
</script>

<style scoped>
.external-tools-container {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--bg-dark-800);
  border-radius: 12px;
  overflow: hidden;
}

.tool-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border-dark);
  background: var(--bg-dark-700);
}

.tool-header h2 {
  margin: 0;
  font-size: 1.25rem;
  font-weight: 600;
  color: var(--text-primary);
}

.tool-actions {
  display: flex;
  gap: 12px;
}

.iframe-container {
  flex: 1;
  position: relative;
  min-height: 600px;
}

.iframe-container iframe {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  border: none;
  background: white;
}

.loading-container,
.fallback-container {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px;
  min-height: 600px;
}

.loading-container p,
.fallback-container p {
  margin: 8px 0 0;
  color: var(--text-secondary);
}

.fallback-icon {
  color: var(--primary-color);
  margin-bottom: 16px;
}

.fallback-content {
  text-align: center;
}

.fallback-content h3 {
  margin: 0 0 8px;
  font-size: 1.5rem;
  color: var(--text-primary);
}

.spinner {
  width: 40px;
  height: 40px;
  border: 4px solid var(--border-dark);
  border-top-color: var(--primary-color);
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
  text-decoration: none;
  border: 1px solid transparent;
}

.btn-primary {
  background: var(--primary-color);
  color: white;
}

.btn-primary:hover {
  background: var(--primary-hover);
}

.btn-outline {
  background: transparent;
  border-color: var(--border-dark);
  color: var(--text-primary);
}

.btn-outline:hover {
  background: var(--bg-dark-600);
}
</style>

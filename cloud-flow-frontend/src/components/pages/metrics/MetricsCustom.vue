<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">自定义指标</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">管理和查看自定义指标</p>
      </div>
      <button class="btn-secondary" @click="fetchData" :disabled="loading">
        <RefreshCw class="w-4 h-4" :class="{ 'animate-spin': loading }" />
        刷新
      </button>
    </div>

    <div class="card p-4">
      <div class="flex items-center gap-3">
        <div class="relative flex-1">
          <Search class="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
          <input
            v-model="searchKeyword"
            type="text"
            placeholder="按指标名称搜索..."
            class="w-full pl-10 pr-4 py-2 bg-slate-50 dark:bg-dark-700 border border-slate-200 dark:border-dark-600 rounded-lg text-sm text-slate-900 dark:text-white placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
          />
        </div>
        <div class="text-sm text-slate-500">共 {{ filteredMetrics.length }} 条</div>
      </div>
    </div>

    <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">指标总数</p>
        <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">{{ customMetrics.length }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">类型数量</p>
        <p class="text-2xl font-bold text-primary-500 mt-1">{{ uniqueTypes }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">最高值</p>
        <p class="text-2xl font-bold text-emerald-500 mt-1">{{ maxValue }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">最低值</p>
        <p class="text-2xl font-bold text-amber-500 mt-1">{{ minValue }}</p>
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">指标列表</h3>
      <div v-if="loading" class="space-y-3">
        <div v-for="i in 4" :key="i" class="h-16 bg-slate-100 dark:bg-dark-700 rounded-lg animate-pulse"></div>
      </div>
      <div v-else-if="!filteredMetrics.length" class="h-40 flex items-center justify-center">
        <p class="text-sm text-slate-400 dark:text-slate-500">暂无数据</p>
      </div>
      <div v-else class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="text-left text-slate-500 dark:text-slate-400 border-b border-slate-200 dark:border-dark-700">
              <th class="pb-3 pr-4 font-medium">指标名</th>
              <th class="pb-3 pr-4 font-medium">类型</th>
              <th class="pb-3 pr-4 font-medium">当前值</th>
              <th class="pb-3 font-medium">描述</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="m in filteredMetrics"
              :key="m.key"
              class="border-b border-slate-100 dark:border-dark-700 hover:bg-slate-50 dark:hover:bg-dark-700/50"
            >
              <td class="py-3 pr-4 font-medium text-slate-900 dark:text-white">{{ m.name }}</td>
              <td class="py-3 pr-4">
                <span class="text-xs px-2 py-0.5 rounded-full bg-primary-100 text-primary-600 dark:bg-primary-500/20 dark:text-primary-400">
                  {{ m.type }}
                </span>
              </td>
              <td class="py-3 pr-4 text-slate-700 dark:text-slate-200 font-mono">{{ m.value }}</td>
              <td class="py-3 text-slate-500 dark:text-slate-400">{{ m.description }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { RefreshCw, Search } from 'lucide-vue-next'
import { queryService } from '../../../api'

const loading = ref(true)
const searchKeyword = ref('')
const customMetrics = ref([])

const pick = (obj, keys) => {
  if (!obj) return null
  for (const k of keys) {
    if (obj[k] !== undefined && obj[k] !== null) return obj[k]
  }
  return null
}

const filteredMetrics = computed(() => {
  const kw = searchKeyword.value.trim().toLowerCase()
  if (!kw) return customMetrics.value
  return customMetrics.value.filter((m) =>
    m.name.toLowerCase().includes(kw) ||
    m.type.toLowerCase().includes(kw) ||
    m.description.toLowerCase().includes(kw)
  )
})

const uniqueTypes = computed(() => {
  const types = new Set(customMetrics.value.map((m) => m.type))
  return types.size
})

const maxValue = computed(() => {
  const nums = customMetrics.value
    .map((m) => Number(m.value))
    .filter((v) => !Number.isNaN(v))
  return nums.length ? Math.max(...nums) : 0
})

const minValue = computed(() => {
  const nums = customMetrics.value
    .map((m) => Number(m.value))
    .filter((v) => !Number.isNaN(v))
  return nums.length ? Math.min(...nums) : 0
})

const parseCustomMetrics = (raw) => {
  if (!raw) return []
  const list = Array.isArray(raw) ? raw : pick(raw, ['metrics', 'custom_metrics', 'customMetrics', 'data', 'items', 'results']) || []
  if (!Array.isArray(list)) return []

  return list.map((m, idx) => ({
    key: pick(m, ['id', 'metric_id', 'metricId', 'name']) || `metric-${idx}`,
    name: pick(m, ['metric_name', 'metric', 'name', 'key']) || 'Unknown',
    type: String(pick(m, ['type', 'metric_type', 'metricType', 'category']) || 'custom'),
    value: pick(m, ['value', 'val', 'current_value', 'currentValue']) ?? 0,
    description: String(pick(m, ['description', 'desc', 'remark', 'comment']) || '-'),
  }))
}

const fetchData = async () => {
  loading.value = true
  try {
    const [metricsRes, overviewRes] = await Promise.allSettled([
      queryService.getMetrics({ limit: 100, type: 'custom' }),
      queryService.getOverview(),
    ])

    const metrics = metricsRes.status === 'fulfilled' ? metricsRes.value : null
    const overview = overviewRes.status === 'fulfilled' ? overviewRes.value : null

    const listFromMetrics = metrics ? parseCustomMetrics(metrics) : []
    const listFromOverview = overview ? parseCustomMetrics(pick(overview, ['metrics', 'custom_metrics', 'data'])) : []

    if (listFromMetrics.length) {
      customMetrics.value = listFromMetrics
    } else if (listFromOverview.length) {
      customMetrics.value = listFromOverview
    } else {
      customMetrics.value = []
    }
  } catch (err) {
    console.warn('MetricsCustom fetch error:', err)
    customMetrics.value = []
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>

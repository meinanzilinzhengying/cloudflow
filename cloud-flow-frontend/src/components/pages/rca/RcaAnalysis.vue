<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">根因分析</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">智能分析故障根因</p>
      </div>
      <button class="btn-primary" @click="fetchData">
        <Zap class="w-4 h-4" />
        分析
      </button>
    </div>

    <div class="grid grid-cols-2 gap-4">
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">分析总数</p>
            <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">{{ totalAnalysis }}</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-primary-50 dark:bg-primary-500/10 flex items-center justify-center">
            <BarChart3 class="w-5 h-5 text-primary-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">平均置信度</p>
            <p class="text-2xl font-bold text-emerald-500 mt-1">{{ avgConfidence }}%</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-emerald-50 dark:bg-emerald-500/10 flex items-center justify-center">
            <Target class="w-5 h-5 text-emerald-500" />
          </div>
        </div>
      </div>
    </div>

    <div class="card p-6">
      <div v-if="loading" class="flex items-center justify-center py-12">
        <Loader2 class="w-8 h-8 text-primary-500 animate-spin" />
      </div>

      <div v-else-if="rootCauses.length === 0" class="flex flex-col items-center justify-center py-12 text-slate-500">
        <Inbox class="w-12 h-12 mb-3 text-slate-300" />
        <p>暂无数据</p>
      </div>

      <div v-else class="space-y-4">
        <div
          v-for="(cause, idx) in rootCauses"
          :key="idx"
          class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl"
        >
          <div class="flex items-start justify-between">
            <div class="flex-1">
              <div class="flex items-center gap-2 mb-2">
                <span class="text-xs px-2 py-1 rounded-full bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-400">
                  置信度 {{ cause.confidence }}%
                </span>
              </div>
              <p class="font-semibold text-slate-900 dark:text-white">{{ cause.description }}</p>
              <p v-if="cause.explanation" class="text-sm text-slate-500 mt-1">{{ cause.explanation }}</p>
              <div v-if="cause.affected_services && cause.affected_services.length > 0" class="mt-3 flex flex-wrap gap-2">
                <span
                  v-for="(svc, i) in cause.affected_services"
                  :key="i"
                  class="text-xs px-2 py-1 rounded bg-primary-50 text-primary-700 dark:bg-primary-500/10 dark:text-primary-400"
                >
                  {{ svc }}
                </span>
              </div>
            </div>
            <div class="text-right ml-4">
              <div class="w-16 h-16 rounded-full bg-emerald-50 dark:bg-emerald-500/10 flex items-center justify-center">
                <span class="text-lg font-bold text-emerald-600 dark:text-emerald-400">{{ cause.confidence }}%</span>
              </div>
            </div>
          </div>
          <div v-if="cause.time" class="mt-3 pt-3 border-t border-slate-200 dark:border-dark-600 text-xs text-slate-500">
            {{ cause.time }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Zap, Target, BarChart3, Loader2, Inbox } from 'lucide-vue-next'
import { queryService } from '../../../api'

const loading = ref(false)
const rootCauses = ref([])
const abnormalServices = ref([])
const abnormalTraces = ref([])
const abnormalLogs = ref([])
const abnormalFlows = ref([])

const totalAnalysis = computed(() => rootCauses.value.length)
const avgConfidence = computed(() => {
  if (rootCauses.value.length === 0) return 0
  const sum = rootCauses.value.reduce((acc, c) => acc + (Number(c.confidence) || 0), 0)
  return Math.round(sum / rootCauses.value.length)
})

const normalizeItem = (item) => ({
  ...item,
  root_cause: item.root_cause || item.rootCause,
  description: item.description || item.message || item.root_cause || item.rootCause || '',
  explanation: item.explanation || item.description || '',
  confidence: item.confidence ?? item.score ?? 0,
  affected_services: item.affected_services || item.affectedServices || [],
  time: item.time || item.timestamp || '',
  events: item.events || [],
  traces: item.traces || [],
  logs: item.logs || [],
})

const fetchData = async () => {
  loading.value = true
  try {
    const data = await queryService.getRCA({ limit: 20 })
    const list = Array.isArray(data) ? data : (data.data || data.items || data.results || data.rca || [])
    rootCauses.value = list.map(normalizeItem)
  } catch (err) {
    rootCauses.value = []
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>

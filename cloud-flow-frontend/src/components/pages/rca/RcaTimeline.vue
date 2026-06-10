<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">故障时间线</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">查看故障发生的时间线</p>
      </div>
    </div>

    <div class="card p-6">
      <div v-if="loading" class="flex items-center justify-center py-12">
        <Loader2 class="w-8 h-8 text-primary-500 animate-spin" />
      </div>

      <div v-else-if="timelineEvents.length === 0" class="flex flex-col items-center justify-center py-12 text-slate-500">
        <Inbox class="w-12 h-12 mb-3 text-slate-300" />
        <p>暂无数据</p>
      </div>

      <div v-else class="relative pl-8">
        <div class="absolute left-3 top-0 bottom-0 w-0.5 bg-slate-200 dark:bg-dark-700"></div>
        <div
          v-for="(event, idx) in timelineEvents"
          :key="idx"
          class="relative mb-6 last:mb-0"
        >
          <div :class="['absolute left-0 top-1 w-6 h-6 rounded-full border-2 flex items-center justify-center', eventTypeClass(event)]">
            <AlertTriangle v-if="event.type === 'incident'" class="w-3 h-3 text-red-500" />
            <Search v-else-if="event.type === 'detect'" class="w-3 h-3 text-amber-500" />
            <CheckCircle v-else class="w-3 h-3 text-green-500" />
          </div>
          <div class="ml-4 p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
            <div class="flex items-center justify-between">
              <h4 class="font-semibold text-slate-900 dark:text-white">{{ event.title || event.description || '事件' }}</h4>
              <span class="text-xs text-slate-500">{{ event.time || event.timestamp || '' }}</span>
            </div>
            <p v-if="event.description || event.message" class="text-sm text-slate-600 dark:text-slate-300 mt-1">
              {{ event.description || event.message }}
            </p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { AlertTriangle, Search, CheckCircle, Loader2, Inbox } from 'lucide-vue-next'
import { queryService } from '../../../api'

const loading = ref(false)
const timelineEvents = ref([])

const eventTypeClass = (event) => {
  const t = event.type || event.level || event.severity || ''
  if (t === 'incident' || t === 'error' || t === 'critical' || t === 'high') {
    return 'bg-red-100 border-red-500'
  }
  if (t === 'detect' || t === 'warning' || t === 'warn' || t === 'medium') {
    return 'bg-amber-100 border-amber-500'
  }
  return 'bg-green-100 border-green-500'
}

const flattenEvents = (list) => {
  const events = []
  list.forEach((item) => {
    if (Array.isArray(item.events) && item.events.length > 0) {
      item.events.forEach((ev) => {
        events.push({
          ...ev,
          title: ev.title || ev.name || ev.description || item.description || '',
          time: ev.time || ev.timestamp || item.time || item.timestamp || '',
        })
      })
    } else if (Array.isArray(item.logs) && item.logs.length > 0) {
      item.logs.forEach((log) => {
        events.push({
          type: 'detect',
          title: log.message || log.content || item.description || '',
          description: log.service || log.level || '',
          time: log.time || log.timestamp || item.time || item.timestamp || '',
        })
      })
    } else {
      events.push({
        type: item.type || 'detect',
        title: item.description || item.message || item.root_cause || item.rootCause || '',
        description: item.explanation || '',
        time: item.time || item.timestamp || '',
      })
    }
  })
  return events.sort((a, b) => {
    const ta = new Date(a.time || 0).getTime()
    const tb = new Date(b.time || 0).getTime()
    return ta - tb
  })
}

const fetchData = async () => {
  loading.value = true
  try {
    const data = await queryService.getRCA({ limit: 20 })
    const list = Array.isArray(data) ? data : (data.data || data.items || data.results || data.rca || [])
    timelineEvents.value = flattenEvents(list)
  } catch (err) {
    timelineEvents.value = []
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>

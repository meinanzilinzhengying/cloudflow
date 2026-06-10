<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">故障时间线</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">查看故障发生的时间线</p>
      </div>
    </div>

    <div class="card p-6">
      <div class="relative pl-8">
        <div class="absolute left-3 top-0 bottom-0 w-0.5 bg-slate-200 dark:bg-dark-700"></div>
        <div
          v-for="event in timelineEvents"
          :key="event.id"
          class="relative mb-6 last:mb-0"
        >
          <div :class="['absolute left-0 top-1 w-6 h-6 rounded-full border-2 flex items-center justify-center', event.type === 'incident' ? 'bg-red-100 border-red-500' : event.type === 'detect' ? 'bg-amber-100 border-amber-500' : 'bg-green-100 border-green-500']">
            <AlertTriangle v-if="event.type === 'incident'" class="w-3 h-3 text-red-500" />
            <Search v-else-if="event.type === 'detect'" class="w-3 h-3 text-amber-500" />
            <CheckCircle v-else class="w-3 h-3 text-green-500" />
          </div>
          <div class="ml-4 p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
            <div class="flex items-center justify-between">
              <h4 class="font-semibold text-slate-900 dark:text-white">{{ event.title }}</h4>
              <span class="text-xs text-slate-500">{{ event.time }}</span>
            </div>
            <p class="text-sm text-slate-600 dark:text-slate-300 mt-1">{{ event.description }}</p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { AlertTriangle, Search, CheckCircle } from 'lucide-vue-next'

const timelineEvents = ref([
  { id: 1, type: 'detect', title: '检测到延迟增加', time: '10:25:00', description: 'API Gateway延迟从50ms增加到150ms' },
  { id: 2, type: 'detect', title: '检测到错误率上升', time: '10:27:00', description: 'Order Service错误率从1%上升到5%' },
  { id: 3, type: 'incident', title: '触发告警', time: '10:30:00', description: '订单服务响应延迟增加告警触发' },
  { id: 4, type: 'detect', title: '定位到Redis延迟', time: '10:31:00', description: 'Redis响应时间达到200ms' },
  { id: 5, type: 'detect', title: '定位到连接池问题', time: '10:32:00', description: 'MySQL连接池使用率达到100%' },
  { id: 6, type: 'resolve', title: '根因分析完成', time: '10:35:00', description: '分析完成，建议扩容Redis和增加连接池' },
])
</script>

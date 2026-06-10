<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">事件关联</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">关联分析相关事件</p>
      </div>
    </div>

    <div class="card p-6">
      <div class="h-80">
        <ECharts :option="correlationOption" class="w-full h-full" autoresize />
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { GraphChart } from 'echarts/charts'
import { TooltipComponent } from 'echarts/components'
import ECharts from 'vue-echarts'

use([CanvasRenderer, GraphChart, TooltipComponent])

const correlationOption = computed(() => ({
  tooltip: {
    trigger: 'item',
    backgroundColor: 'rgba(255,255,255,0.95)',
    textStyle: { color: '#1e293b' },
  },
  series: [{
    type: 'graph',
    layout: 'force',
    symbolSize: (data) => data.size || 40,
    roam: true,
    label: { show: true, fontSize: 12, color: '#64748b' },
    lineStyle: { width: 2, color: '#cbd5e1', curveness: 0.2 },
    emphasis: { focus: 'adjacency' },
    force: {
      repulsion: 300,
      gravity: 0.1,
      edgeLength: [80, 150],
    },
    data: [
      { name: '告警事件', size: 50, itemStyle: { color: '#ef4444' } },
      { name: '慢Trace', size: 40, itemStyle: { color: '#f59e0b' } },
      { name: '错误日志', size: 40, itemStyle: { color: '#ef4444' } },
      { name: '异常流量', size: 40, itemStyle: { color: '#3b82f6' } },
      { name: 'Redis延迟', size: 45, itemStyle: { color: '#8b5cf6' } },
      { name: 'MySQL连接池', size: 45, itemStyle: { color: '#14b8a6' } },
      { name: '网络带宽', size: 40, itemStyle: { color: '#2563eb' } },
    ],
    links: [
      { source: '告警事件', target: '慢Trace', value: 0.9 },
      { source: '告警事件', target: '错误日志', value: 0.85 },
      { source: '告警事件', target: '异常流量', value: 0.7 },
      { source: '慢Trace', target: 'Redis延迟', value: 0.92 },
      { source: '慢Trace', target: 'MySQL连接池', value: 0.86 },
      { source: '错误日志', target: 'MySQL连接池', value: 0.8 },
      { source: '异常流量', target: '网络带宽', value: 0.75 },
    ],
  }],
}))
</script>

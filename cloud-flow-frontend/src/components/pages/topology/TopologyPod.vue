<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">Pod拓扑</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">Pod级别的依赖关系</p>
      </div>
    </div>

    <div class="card p-6">
      <div class="h-[500px]">
        <ECharts :option="podTopologyOption" class="w-full h-full" autoresize />
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

const podTopologyOption = computed(() => ({
  tooltip: {
    trigger: 'item',
    backgroundColor: 'rgba(255,255,255,0.95)',
    textStyle: { color: '#1e293b' },
  },
  series: [{
    type: 'graph',
    layout: 'force',
    symbolSize: 30,
    roam: true,
    label: { show: true, position: 'bottom', fontSize: 10, color: '#64748b' },
    lineStyle: { width: 1.5, color: '#e2e8f0', curveness: 0.1 },
    data: [
      { name: 'api-gw-0', itemStyle: { color: '#22c55e' } },
      { name: 'api-gw-1', itemStyle: { color: '#22c55e' } },
      { name: 'user-0', itemStyle: { color: '#22c55e' } },
      { name: 'user-1', itemStyle: { color: '#f59e0b' } },
      { name: 'order-0', itemStyle: { color: '#22c55e' } },
      { name: 'order-1', itemStyle: { color: '#22c55e' } },
      { name: 'order-2', itemStyle: { color: '#22c55e' } },
      { name: 'pay-0', itemStyle: { color: '#22c55e' } },
      { name: 'db-primary', itemStyle: { color: '#2563eb' } },
      { name: 'db-replica', itemStyle: { color: '#3b82f6' } },
    ],
    links: [
      { source: 'api-gw-0', target: 'user-0' },
      { source: 'api-gw-0', target: 'order-0' },
      { source: 'api-gw-1', target: 'user-1' },
      { source: 'api-gw-1', target: 'order-1' },
      { source: 'user-0', target: 'db-primary' },
      { source: 'user-1', target: 'db-replica' },
      { source: 'order-0', target: 'pay-0' },
      { source: 'order-1', target: 'pay-0' },
      { source: 'order-2', target: 'pay-0' },
      { source: 'order-0', target: 'db-primary' },
      { source: 'order-1', target: 'db-replica' },
      { source: 'pay-0', target: 'db-primary' },
    ],
  }],
}))
</script>

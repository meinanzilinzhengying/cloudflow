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

const podTopologyOption = computed(() => ({ tooltip: { trigger: 'item' }, series: [{ type: 'graph', layout: 'force', data: [], links: [] }] }))
</script>

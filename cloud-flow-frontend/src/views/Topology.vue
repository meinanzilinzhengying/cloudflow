<template>
  <div class="topology">
    <div class="page-header">
      <h2 class="page-title">网络拓扑</h2>
      <div class="header-actions">
        <el-select v-model="viewMode" placeholder="视图" size="small" style="width: 120px">
          <el-option label="服务拓扑" value="service" />
          <el-option label="主机拓扑" value="host" />
          <el-option label="Pod 拓扑" value="pod" />
        </el-select>
        <el-button type="primary" size="small" @click="refresh">刷新</el-button>
      </div>
    </div>
    <el-card class="chart-card" :body-style="{ padding: '20px' }">
      <div ref="topologyRef" style="height: 600px"></div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import * as echarts from 'echarts'

const viewMode = ref('service')
const topologyRef = ref<HTMLDivElement | null>(null)

const refresh = () => {
  if (!topologyRef.value) return
  const chart = echarts.init(topologyRef.value)
  chart.setOption({
    tooltip: {},
    series: [{
      type: 'graph', layout: 'force', roam: true,
      label: { show: true },
      force: { repulsion: 200, edgeLength: 100 },
      data: [
        { name: 'Gateway', symbolSize: 50, itemStyle: { color: '#165DFF' } },
        { name: 'LB', symbolSize: 40, itemStyle: { color: '#165DFF' } },
        { name: 'Web-1', symbolSize: 30, itemStyle: { color: '#67C23A' } },
        { name: 'Web-2', symbolSize: 30, itemStyle: { color: '#67C23A' } },
        { name: 'DB-1', symbolSize: 35, itemStyle: { color: '#E6A23C' } },
        { name: 'Cache', symbolSize: 25, itemStyle: { color: '#67C23A' } },
        { name: 'MQ', symbolSize: 25, itemStyle: { color: '#67C23A' } },
        { name: 'API', symbolSize: 30, itemStyle: { color: '#67C23A' } },
      ],
      links: [
        { source: 'Gateway', target: 'LB' },
        { source: 'LB', target: 'Web-1' },
        { source: 'LB', target: 'Web-2' },
        { source: 'Web-1', target: 'API' },
        { source: 'Web-2', target: 'API' },
        { source: 'API', target: 'DB-1' },
        { source: 'API', target: 'Cache' },
        { source: 'API', target: 'MQ' },
      ]
    }]
  })
  window.addEventListener('resize', () => chart.resize())
}

onMounted(refresh)
</script>

<style scoped lang="scss">
.topology {
  .page-header {
    display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;
    .page-title { font-size: 18px; font-weight: 600; color: var(--el-text-color-primary); }
    .header-actions { display: flex; gap: 12px; align-items: center; }
  }
  .chart-card {
    border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.08);
  }
}
</style>

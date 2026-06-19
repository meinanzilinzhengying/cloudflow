<template>
  <div class="topology-page">
    <div class="page-header">
      <h2 class="page-title">业务拓扑</h2>
      <div class="header-actions">
        <el-select v-model="viewMode" placeholder="视图" size="small" class="dark-select">
          <el-option label="服务拓扑" value="service" />
          <el-option label="主机拓扑" value="host" />
          <el-option label="Pod 拓扑" value="pod" />
        </el-select>
        <el-button type="primary" size="small" class="dark-btn" @click="refresh">刷新</el-button>
      </div>
    </div>
    <div class="topology-chart" ref="topologyRef"></div>
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
    backgroundColor: 'transparent',
    tooltip: {
      backgroundColor: 'rgba(5, 56, 90, 0.9)',
      borderColor: '#0ABAFF',
      textStyle: { color: '#fff' }
    },
    series: [{
      type: 'graph', layout: 'force', roam: true,
      label: {
        show: true,
        color: '#fff',
        fontSize: 12
      },
      force: { repulsion: 300, edgeLength: [80, 200] },
      data: [
        { name: 'Gateway', symbolSize: 60, itemStyle: { color: '#00CCFF' }, label: { color: '#fff' } },
        { name: 'LB', symbolSize: 50, itemStyle: { color: '#0ABAFF' }, label: { color: '#fff' } },
        { name: 'Web-1', symbolSize: 40, itemStyle: { color: '#6BEDB7' }, label: { color: '#fff' } },
        { name: 'Web-2', symbolSize: 40, itemStyle: { color: '#6BEDB7' }, label: { color: '#fff' } },
        { name: 'DB-1', symbolSize: 45, itemStyle: { color: '#FF745A' }, label: { color: '#fff' } },
        { name: 'Cache', symbolSize: 35, itemStyle: { color: '#FFC328' }, label: { color: '#fff' } },
        { name: 'MQ', symbolSize: 35, itemStyle: { color: '#6BEDB7' }, label: { color: '#fff' } },
        { name: 'API', symbolSize: 40, itemStyle: { color: '#00CCFF' }, label: { color: '#fff' } },
      ],
      links: [
        { source: 'Gateway', target: 'LB', lineStyle: { color: 'rgba(0, 204, 255, 0.6)', width: 2 } },
        { source: 'LB', target: 'Web-1', lineStyle: { color: 'rgba(0, 204, 255, 0.6)', width: 2 } },
        { source: 'LB', target: 'Web-2', lineStyle: { color: 'rgba(0, 204, 255, 0.6)', width: 2 } },
        { source: 'Web-1', target: 'API', lineStyle: { color: 'rgba(0, 204, 255, 0.6)', width: 2 } },
        { source: 'Web-2', target: 'API', lineStyle: { color: 'rgba(0, 204, 255, 0.6)', width: 2 } },
        { source: 'API', target: 'DB-1', lineStyle: { color: 'rgba(0, 204, 255, 0.6)', width: 2 } },
        { source: 'API', target: 'Cache', lineStyle: { color: 'rgba(0, 204, 255, 0.6)', width: 2 } },
        { source: 'API', target: 'MQ', lineStyle: { color: 'rgba(0, 204, 255, 0.6)', width: 2 } },
      ]
    }]
  })
  window.addEventListener('resize', () => chart.resize())
}

onMounted(refresh)
</script>

<style scoped lang="scss">
.topology-page {
  min-height: 100vh;
  padding: 20px 24px;
  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 20px;
    .page-title {
      font-size: 18px;
      font-weight: 600;
      color: #FFFFFF;
    }
    .header-actions {
      display: flex;
      gap: 12px;
      align-items: center;
    }
  }
  .topology-chart {
    height: 600px;
    background: rgba(10, 186, 255, 0.08);
    border: 1px solid rgba(10, 186, 255, 0.3);
    border-radius: 8px;
  }
}
</style>

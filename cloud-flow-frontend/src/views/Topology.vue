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
import axios from 'axios'

const viewMode = ref('service')
const topologyRef = ref<HTMLDivElement | null>(null)
const chartInstance = ref<echarts.ECharts | null>(null)

const api = axios.create({ baseURL: '/api', timeout: 30000 })

const refresh = async () => {
  if (!topologyRef.value) return

  let chart = chartInstance.value
  if (!chart) {
    chart = echarts.init(topologyRef.value)
    chartInstance.value = chart
    window.addEventListener('resize', () => chart!.resize())
  }

  try {
    const res = await api.get('/topology')
    const data = res.data?.data || res.data || {}
    const nodes = data.nodes || []
    const links = data.links || []

    if (nodes.length === 0) {
      chart.setOption({
        backgroundColor: 'transparent',
        title: { text: '暂无拓扑数据', left: 'center', top: 'center', textStyle: { color: '#fff', fontSize: 16 } },
        series: []
      }, true)
      return
    }

    chart.setOption({
      backgroundColor: 'transparent',
      tooltip: {
        backgroundColor: 'rgba(5, 56, 90, 0.9)',
        borderColor: '#0ABAFF',
        textStyle: { color: '#fff' }
      },
      series: [{
        type: 'graph', layout: 'force', roam: true,
        label: { show: true, color: '#fff', fontSize: 12 },
        force: { repulsion: 300, edgeLength: [80, 200] },
        data: nodes,
        links: links
      }]
    }, true)
  } catch (err) {
    console.error('获取拓扑数据失败:', err)
    chart.setOption({
      backgroundColor: 'transparent',
      title: { text: '加载失败', left: 'center', top: 'center', textStyle: { color: '#FF745A', fontSize: 16 } },
      series: []
    }, true)
  }
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

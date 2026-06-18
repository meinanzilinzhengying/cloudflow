<template>
  <div class="topology">
    <div class="page-header">
      <h2 class="page-title">网络拓扑</h2>
      <div class="header-actions">
        <el-button type="primary" size="small" @click="fetchTopology">刷新</el-button>
        <el-tag type="success" size="small">实时数据</el-tag>
      </div>
    </div>
    <el-card class="chart-card" :body-style="{ padding: '20px' }">
      <div class="card-title">
        通信拓扑图
        <span class="sub-tip">节点大小代表流量，连线粗细代表连接数</span>
      </div>
      <div ref="topologyRef" style="height: 580px"></div>
    </el-card>
    <el-row :gutter="16" class="bottom-row">
      <el-col :span="12">
        <el-card class="info-card" :body-style="{ padding: '16px' }">
          <div class="card-title">活跃节点</div>
          <el-table :data="nodeList" size="small" max-height="200">
            <el-table-column prop="name" label="主机/IP" />
            <el-table-column prop="connections" label="连接数" width="80" />
            <el-table-column prop="bytes" label="流量" width="100">
              <template #default="{ row }">{{ formatBytes(row.bytes || 0) }}</template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card class="info-card" :body-style="{ padding: '16px' }">
          <div class="card-title">活跃连接</div>
          <el-table :data="edgeList" size="small" max-height="200">
            <el-table-column prop="source" label="源" width="130" />
            <el-table-column prop="target" label="目的" width="130" />
            <el-table-column prop="value" label="连接数" width="80" />
          </el-table>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import * as echarts from 'echarts'
import { getNetworkTopology, getNetworkFlows } from '@/api/network'
import { formatBytes } from '@/utils/format'

const topologyRef = ref<HTMLDivElement | null>(null)
let chart: echarts.ECharts | null = null

interface NodeInfo { name: string; connections: number; bytes: number }
interface EdgeInfo { source: string; target: string; value: number }

const nodeList = ref<NodeInfo[]>([])
const edgeList = ref<EdgeInfo[]>([])

const buildTopologyFromFlows = (flows: any[]) => {
  const nodeMap: Record<string, NodeInfo> = {}
  const edgeMap: Record<string, EdgeInfo> = {}

  flows.forEach(f => {
    const src = f.src || f.srcIp || ''
    const dst = f.dst || f.dstIp || ''
    const bytes = f.bytes || 0
    if (!src || !dst) return

    if (!nodeMap[src]) nodeMap[src] = { name: src, connections: 0, bytes: 0 }
    if (!nodeMap[dst]) nodeMap[dst] = { name: dst, connections: 0, bytes: 0 }
    nodeMap[src].connections++
    nodeMap[src].bytes += bytes
    nodeMap[dst].bytes += bytes

    const key = `${src}→${dst}`
    if (!edgeMap[key]) edgeMap[key] = { source: src, target: dst, value: 0 }
    edgeMap[key].value++
  })

  const nodes = Object.values(nodeMap)
  const edges = Object.values(edgeMap)
  nodeList.value = nodes.sort((a, b) => b.bytes - a.bytes).slice(0, 10)
  edgeList.value = edges.sort((a, b) => b.value - a.value).slice(0, 10)

  const maxBytes = Math.max(...nodes.map(n => n.bytes), 1)
  return {
    nodes: nodes.map(n => ({
      name: n.name,
      symbolSize: Math.max(15, Math.min(50, 15 + (n.bytes / maxBytes) * 35)),
      label: { show: true, fontSize: 10 }
    })),
    edges: edges.map(e => ({
      source: e.source,
      target: e.target,
      lineStyle: { width: Math.max(1, Math.min(6, e.value)) }
    }))
  }
}

const fetchTopology = async () => {
  try {
    // 优先使用topology端点，否则用flows推导
    let graphData = { nodes: [] as any[], edges: [] as any[] }
    try {
      const topoRes = await getNetworkTopology()
      if (topoRes.code === 0 && topoRes.data && (topoRes.data as any).nodes) {
        graphData = topoRes.data as any
      }
    } catch {}

    if (!graphData.nodes.length) {
      const flowRes = await getNetworkFlows({ range: '1h' })
      if (flowRes.code === 0 && Array.isArray(flowRes.data) && flowRes.data.length) {
        graphData = buildTopologyFromFlows(flowRes.data)
      }
    }

    if (!chart || !topologyRef.value) return
    chart.setOption({
      tooltip: { formatter: (p: any) => p.data.name || `${p.data.source} → ${p.data.target}` },
      series: [{
        type: 'graph', layout: 'force', roam: true,
        label: { show: true, fontSize: 10 },
        edgeSymbol: ['none', 'arrow'],
        edgeSymbolSize: [0, 8],
        data: graphData.nodes,
        links: graphData.edges,
        force: { repulsion: 200, edgeLength: [60, 120], gravity: 0.1 },
        itemStyle: { color: '#409EFF', borderColor: '#fff', borderWidth: 2 },
        lineStyle: { color: '#aaa', curveness: 0.1 }
      }]
    })
  } catch (e) { console.error('Topology fetch error:', e) }
}

onMounted(() => {
  if (topologyRef.value) {
    chart = echarts.init(topologyRef.value)
    window.addEventListener('resize', () => chart?.resize())
  }
  fetchTopology()
})
onUnmounted(() => { chart?.dispose() })
</script>

<style scoped lang="scss">
.topology {
  .page-header {
    display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;
    .page-title { font-size: 18px; font-weight: 600; color: var(--el-text-color-primary); }
    .header-actions { display: flex; gap: 12px; align-items: center; }
  }
  .chart-card {
    border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.08); margin-bottom: 16px;
    .card-title {
      font-size: 14px; font-weight: 600; color: var(--el-text-color-primary); margin-bottom: 12px; display: flex; align-items: center; gap: 12px;
      .sub-tip { font-size: 12px; font-weight: 400; color: var(--el-text-color-secondary); }
    }
  }
  .bottom-row {}
  .info-card {
    border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.08);
    .card-title { font-size: 14px; font-weight: 600; color: var(--el-text-color-primary); margin-bottom: 8px; }
  }
}
</style>

<template>
  <div class="network">
    <div class="page-header">
      <h2 class="page-title">网络流量分析</h2>
      <div class="header-actions">
        <TimePicker v-model="timeRange" @change="fetchData" />
        <el-input v-model="filterText" placeholder="按IP/端口/协议筛选" size="small" clearable style="width: 220px" />
        <el-button type="primary" size="small" @click="fetchData">刷新</el-button>
      </div>
    </div>

    <!-- 流量趋势 -->
    <el-card class="chart-card" :body-style="{ padding: '20px' }">
      <div class="card-title">
        流量趋势
        <el-tag size="small" type="info" style="margin-left:8px">{{ trends.length }} 个时间点</el-tag>
      </div>
      <v-chart :option="flowTrendOption" autoresize style="height: 300px" />
    </el-card>

    <!-- 通信矩阵 + 拓扑 -->
    <el-row :gutter="24" class="matrix-row">
      <el-col :span="12">
        <el-card class="chart-card" :body-style="{ padding: '20px' }">
          <div class="card-title">协议流量分布</div>
          <v-chart :option="protocolBarOption" autoresize style="height: 280px" />
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card class="chart-card" :body-style="{ padding: '20px' }">
          <div class="card-title">实时 PPS (包/秒)</div>
          <v-chart :option="ppsOption" autoresize style="height: 280px" />
        </el-card>
      </el-col>
    </el-row>

    <!-- 流日志表 -->
    <el-card class="table-card" :body-style="{ padding: '20px' }">
      <div class="card-title">
        流日志
        <el-tag size="small" type="success" style="margin-left:8px">{{ filteredFlows.length }} 条</el-tag>
      </div>
      <el-table :data="pagedFlows" size="small" style="width: 100%" max-height="400">
        <el-table-column label="时间" width="155">
          <template #default="{ row }">{{ row.time || row.timestamp }}</template>
        </el-table-column>
        <el-table-column label="源IP" width="130">
          <template #default="{ row }">{{ row.src || row.srcIp }}</template>
        </el-table-column>
        <el-table-column label="目的IP" width="130">
          <template #default="{ row }">{{ row.dst || row.dstIp }}</template>
        </el-table-column>
        <el-table-column label="源端口" width="80">
          <template #default="{ row }">{{ row.sport || row.srcPort }}</template>
        </el-table-column>
        <el-table-column label="目的端口" width="80">
          <template #default="{ row }">{{ row.dport || row.dstPort }}</template>
        </el-table-column>
        <el-table-column prop="protocol" label="协议" width="80">
          <template #default="{ row }">
            <el-tag size="small" :type="protoColor(row.protocol)">{{ row.protocol || 'TCP' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="字节数" width="100">
          <template #default="{ row }">{{ formatBytes(row.bytes || 0) }}</template>
        </el-table-column>
        <el-table-column prop="packets" label="包数" width="70" />
        <el-table-column prop="status" label="状态" width="80">
          <template #default="{ row }">
            <el-tag v-if="row.status" size="small" type="success">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
      </el-table>
      <el-pagination
        class="pagination"
        background layout="total, prev, pager, next"
        :total="filteredFlows.length"
        :page-size="pageSize"
        v-model:current-page="currentPage"
      />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import TimePicker from '@/components/TimePicker.vue'
import { getNetworkFlows, getNetworkTrends } from '@/api/network'
import type { FlowRecord, NetworkTrend } from '@/api/network'
import { formatBytes } from '@/utils/format'

const timeRange = ref('1h')
const filterText = ref('')
const flows = ref<FlowRecord[]>([])
const trends = ref<NetworkTrend[]>([])
const currentPage = ref(1)
const pageSize = 20

const protoColor = (proto: string) => {
  const map: Record<string, string> = { HTTP: 'primary', TCP: 'success', UDP: 'warning', DNS: 'info' }
  return map[proto] || ''
}

const filteredFlows = computed(() => {
  if (!filterText.value) return flows.value
  const f = filterText.value.toLowerCase()
  return flows.value.filter(r => {
    const src = (r.src || r.srcIp || '').toLowerCase()
    const dst = (r.dst || r.dstIp || '').toLowerCase()
    const proto = (r.protocol || '').toLowerCase()
    const sp = String(r.sport || r.srcPort || '')
    const dp = String(r.dport || r.dstPort || '')
    return src.includes(f) || dst.includes(f) || proto.includes(f) || sp.includes(f) || dp.includes(f)
  })
})

const pagedFlows = computed(() => {
  const start = (currentPage.value - 1) * pageSize
  return filteredFlows.value.slice(start, start + pageSize)
})

// 协议统计
const protocolStats = computed(() => {
  const map: Record<string, number> = {}
  flows.value.forEach(f => {
    const p = f.protocol || 'TCP'
    map[p] = (map[p] || 0) + (f.bytes || 0)
  })
  return Object.entries(map)
    .sort((a, b) => b[1] - a[1])
    .map(([name, value]) => ({ name, value }))
})

const flowTrendOption = computed(() => ({
  tooltip: {
    trigger: 'axis',
    formatter: (p: any[]) => p.map(i => `${i.seriesName}: ${formatBytes(i.value)}`).join('<br>')
  },
  legend: { data: ['上行流量', '下行流量'], right: 0 },
  dataZoom: [{ type: 'inside' }, { type: 'slider', height: 20 }],
  grid: { left: '3%', right: '4%', bottom: '60px', containLabel: true },
  xAxis: { type: 'category', data: trends.value.map(t => t.time), boundaryGap: false },
  yAxis: { type: 'value', axisLabel: { formatter: (v: number) => formatBytes(v) } },
  series: [
    { name: '上行流量', type: 'line', areaStyle: { opacity: 0.3 }, data: trends.value.map(t => t.tx || 0), smooth: true, lineStyle: { color: '#409EFF' } },
    { name: '下行流量', type: 'line', areaStyle: { opacity: 0.3 }, data: trends.value.map(t => t.rx || 0), smooth: true, lineStyle: { color: '#67C23A' } }
  ]
}))

const protocolBarOption = computed(() => ({
  tooltip: { trigger: 'axis', formatter: (p: any[]) => `${p[0].name}: ${formatBytes(p[0].value)}` },
  xAxis: { type: 'category', data: protocolStats.value.map(p => p.name) },
  yAxis: { type: 'value', axisLabel: { formatter: (v: number) => formatBytes(v) } },
  series: [{
    type: 'bar',
    data: protocolStats.value.map(p => p.value),
    itemStyle: { borderRadius: [4, 4, 0, 0], color: '#409EFF' }
  }]
}))

const ppsOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  xAxis: { type: 'category', data: trends.value.map(t => t.time), boundaryGap: false },
  yAxis: { type: 'value', name: 'pps' },
  series: [{
    name: 'PPS', type: 'line',
    data: trends.value.map(t => t.pps || 0),
    smooth: true, areaStyle: { opacity: 0.2 },
    lineStyle: { color: '#E6A23C' }, itemStyle: { color: '#E6A23C' }
  }]
}))

const fetchData = async () => {
  try {
    const [flowRes, trendRes] = await Promise.all([
      getNetworkFlows({ range: timeRange.value }),
      getNetworkTrends()
    ])
    if (flowRes.code === 0 && Array.isArray(flowRes.data)) flows.value = flowRes.data
    if (trendRes.code === 0 && Array.isArray(trendRes.data)) trends.value = trendRes.data
  } catch (e) { console.error('Network fetch error:', e) }
}

let timer: ReturnType<typeof setInterval> | null = null
onMounted(() => { fetchData(); timer = setInterval(fetchData, 30000) })
onUnmounted(() => { if (timer) clearInterval(timer) })
</script>

<style scoped lang="scss">
.network {
  .page-header {
    display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;
    .page-title { font-size: 18px; font-weight: 600; color: var(--el-text-color-primary); }
    .header-actions { display: flex; gap: 12px; align-items: center; }
  }
  .chart-card {
    border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.08); margin-bottom: 24px;
    .card-title { font-size: 14px; font-weight: 600; color: var(--el-text-color-primary); margin-bottom: 16px; display: flex; align-items: center; }
  }
  .matrix-row { margin-bottom: 24px; }
  .table-card {
    border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.08);
    .card-title { font-size: 14px; font-weight: 600; color: var(--el-text-color-primary); margin-bottom: 12px; display: flex; align-items: center; }
    .pagination { margin-top: 16px; justify-content: flex-end; }
  }
}
</style>

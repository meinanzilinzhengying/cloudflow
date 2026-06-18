<template>
  <div class="network">
    <div class="page-header">
      <h2 class="page-title">网络流量分析</h2>
      <div class="header-actions">
        <TimePicker v-model="timeRange" @change="fetchData" />
        <el-input v-model="filterText" placeholder="按IP/端口/协议筛选" size="small" clearable style="width: 220px" />
      </div>
    </div>
    <el-card class="chart-card" :body-style="{ padding: '20px' }">
      <div class="card-title">流量趋势</div>
      <v-chart :option="flowTrendOption" autoresize style="height: 320px" />
    </el-card>
    <el-row :gutter="24" class="matrix-row">
      <el-col :span="12">
        <el-card class="chart-card" :body-style="{ padding: '20px' }">
          <div class="card-title">通信矩阵</div>
          <v-chart :option="matrixOption" autoresize style="height: 300px" />
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card class="chart-card" :body-style="{ padding: '20px' }">
          <div class="card-title">网络拓扑</div>
          <div ref="topologyChart" style="height: 300px"></div>
        </el-card>
      </el-col>
    </el-row>
    <el-card class="table-card" :body-style="{ padding: '20px' }">
      <div class="card-title">流日志</div>
      <el-table :data="filteredFlows" size="small" style="width: 100%" max-height="400">
        <el-table-column prop="timestamp" label="时间" width="160" />
        <el-table-column prop="srcIp" label="源IP" />
        <el-table-column prop="dstIp" label="目的IP" />
        <el-table-column prop="srcPort" label="源端口" width="80" />
        <el-table-column prop="dstPort" label="目的端口" width="80" />
        <el-table-column prop="protocol" label="协议" width="80" />
        <el-table-column prop="bytes" label="字节数" :formatter="(row: any) => formatBytes(row.bytes)" />
        <el-table-column prop="packets" label="包数" />
        <el-table-column prop="rtt" label="RTT(ms)" width="90" />
      </el-table>
      <el-pagination class="pagination" background layout="total, prev, pager, next" :total="filteredFlows.length" :page-size="20" />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import TimePicker from '@/components/TimePicker.vue'
import { getNetworkFlows } from '@/api/network'
import type { FlowRecord } from '@/api/network'
import { formatBytes } from '@/utils/format'
import * as echarts from 'echarts'

const timeRange = ref('1h')
const filterText = ref('')
const flows = ref<FlowRecord[]>([])
const topologyChart = ref<HTMLDivElement | null>(null)

const filteredFlows = computed(() => {
  if (!filterText.value) return flows.value
  const f = filterText.value.toLowerCase()
  return flows.value.filter(r =>
    r.srcIp.toLowerCase().includes(f) || r.dstIp.toLowerCase().includes(f) ||
    r.protocol.toLowerCase().includes(f) || String(r.srcPort).includes(f) || String(r.dstPort).includes(f)
  )
})

const flowTrendOption = ref({
  backgroundColor: 'transparent',
  tooltip: { trigger: 'axis', backgroundColor: 'rgba(5, 56, 90, 0.9)', borderColor: '#0ABAFF', textStyle: { color: '#fff' } },
  dataZoom: [{ type: 'inside' }, { type: 'slider', backgroundColor: 'rgba(10,186,255,0.1)', fillerColor: 'rgba(0,204,255,0.2)', borderColor: 'transparent', textStyle: { color: '#fff' } }],
  xAxis: { type: 'category', data: ['00:00', '04:00', '08:00', '12:00', '16:00', '20:00'], axisLine: { lineStyle: { color: 'rgba(255,255,255,0.3)' } }, axisLabel: { color: '#fff' }, splitLine: { show: true, lineStyle: { color: 'rgba(255,255,255,0.1)' } } },
  yAxis: { type: 'value', axisLine: { lineStyle: { color: 'rgba(255,255,255,0.3)' } }, axisLabel: { color: '#fff', formatter: (v: number) => formatBytes(v) }, splitLine: { lineStyle: { color: 'rgba(255,255,255,0.1)' } } },
  series: [
    { name: '流量', type: 'line', areaStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: 'rgba(0,204,255,0.4)' }, { offset: 1, color: 'rgba(0,204,255,0.05)' }] } }, itemStyle: { color: '#00CCFF' }, data: [1200000, 1800000, 3500000, 4200000, 3100000, 2500000], smooth: true }
  ]
})

const matrixOption = ref({
  backgroundColor: 'transparent',
  tooltip: { position: 'top', backgroundColor: 'rgba(5, 56, 90, 0.9)', borderColor: '#0ABAFF', textStyle: { color: '#fff' } },
  grid: { height: '70%', top: '10%' },
  xAxis: { type: 'category', data: ['Web', 'DB', 'Cache', 'MQ', 'API'], splitArea: { show: true, areaStyle: { color: ['rgba(10,186,255,0.05)', 'rgba(10,186,255,0.1)'] } }, axisLine: { lineStyle: { color: 'rgba(255,255,255,0.3)' } }, axisLabel: { color: '#fff' } },
  yAxis: { type: 'category', data: ['Web', 'DB', 'Cache', 'MQ', 'API'], splitArea: { show: true, areaStyle: { color: ['rgba(10,186,255,0.05)', 'rgba(10,186,255,0.1)'] } }, axisLine: { lineStyle: { color: 'rgba(255,255,255,0.3)' } }, axisLabel: { color: '#fff' } },
  visualMap: { min: 0, max: 1000, calculable: true, orient: 'horizontal', left: 'center', bottom: '0%', textStyle: { color: '#fff' }, inRange: { color: ['rgba(0,204,255,0.05)', '#0ABAFF', '#00CCFF'] } },
  series: [{
    type: 'heatmap', data: [
      [0,0,0], [1,0,200], [2,0,50], [3,0,100], [4,0,300],
      [0,1,200], [1,1,0], [2,1,80], [3,1,150], [4,1,100],
      [0,2,50], [1,2,80], [2,2,0], [3,2,20], [4,2,60],
      [0,3,100], [1,3,150], [2,3,20], [3,3,0], [4,3,80],
      [0,4,300], [1,4,100], [2,4,60], [3,4,80], [4,4,0],
    ]
  }]
})

const topologyOption = ref({
  backgroundColor: 'transparent',
  tooltip: { backgroundColor: 'rgba(5, 56, 90, 0.9)', borderColor: '#0ABAFF', textStyle: { color: '#fff' } },
  series: [{
    type: 'graph', layout: 'force', roam: true,
    label: { show: true, fontSize: 10, color: '#fff' },
    lineStyle: { color: 'rgba(0,204,255,0.5)', width: 1 },
    force: { repulsion: 200, edgeLength: 120 },
    data: [
      { name: 'Gateway', symbolSize: 45, itemStyle: { color: '#00CCFF' } },
      { name: 'LB', symbolSize: 35, itemStyle: { color: '#0ABAFF' } },
      { name: 'Web-1', symbolSize: 25, itemStyle: { color: '#6BEDB7' } },
      { name: 'Web-2', symbolSize: 25, itemStyle: { color: '#6BEDB7' } },
      { name: 'DB-1', symbolSize: 30, itemStyle: { color: '#FF745A' } },
      { name: 'Cache', symbolSize: 20, itemStyle: { color: '#6BEDB7' } },
      { name: 'MQ', symbolSize: 20, itemStyle: { color: '#6BEDB7' } },
      { name: 'API', symbolSize: 25, itemStyle: { color: '#6BEDB7' } },
    ],
    links: [
      { source: 'Gateway', target: 'LB', lineStyle: { color: 'rgba(0,204,255,0.5)' } },
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

const fetchData = async () => {
  try {
    const res = await getNetworkFlows({ range: timeRange.value })
    if (res.code === 0) flows.value = res.data.flows
    else {
      flows.value = [
        { timestamp: '2026-06-18 14:00:00', srcIp: '192.168.1.101', dstIp: '192.168.1.102', srcPort: 443, dstPort: 54328, protocol: 'TCP', bytes: 15240, packets: 12, rtt: 0.8 },
        { timestamp: '2026-06-18 14:00:01', srcIp: '192.168.1.103', dstIp: '192.168.1.104', srcPort: 3306, dstPort: 49212, protocol: 'TCP', bytes: 8192, packets: 8, rtt: 1.2 },
        { timestamp: '2026-06-18 14:00:02', srcIp: '192.168.1.101', dstIp: '192.168.1.105', srcPort: 53, dstPort: 49152, protocol: 'UDP', bytes: 256, packets: 1, rtt: 0.3 },
      ]
    }
  } catch (e) { /* ignore */ }
}

onMounted(() => {
  fetchData()
  if (topologyChart.value) {
    const chart = echarts.init(topologyChart.value)
    chart.setOption(topologyOption.value)
    window.addEventListener('resize', () => chart.resize())
  }
})
</script>

<style scoped lang="scss">
.network {
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
  .chart-card {
    border-radius: 8px;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
    margin-bottom: 24px;
    .card-title {
      font-size: 16px;
      font-weight: 600;
      color: #FFFFFF;
      margin-bottom: 16px;
    }
  }
  .matrix-row { margin-bottom: 0; }
  .table-card {
    border-radius: 8px;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
    .pagination {
      margin-top: 16px;
      justify-content: flex-end;
    }
  }
}
</style>

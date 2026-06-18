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
          <v-chart :option="topologyOption" autoresize style="height: 300px" />
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

const timeRange = ref('1h')
const filterText = ref('')
const flows = ref<FlowRecord[]>([])

const filteredFlows = computed(() => {
  if (!filterText.value) return flows.value
  const f = filterText.value.toLowerCase()
  return flows.value.filter(r =>
    r.srcIp.toLowerCase().includes(f) || r.dstIp.toLowerCase().includes(f) ||
    r.protocol.toLowerCase().includes(f) || String(r.srcPort).includes(f) || String(r.dstPort).includes(f)
  )
})

const flowTrendOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  dataZoom: [{ type: 'inside' }, { type: 'slider' }],
  xAxis: { type: 'category', data: ['00:00', '04:00', '08:00', '12:00', '16:00', '20:00'] },
  yAxis: { type: 'value', axisLabel: { formatter: (v: number) => formatBytes(v) } },
  series: [
    { name: '流量', type: 'line', areaStyle: { opacity: 0.3 }, data: [1200000, 1800000, 3500000, 4200000, 3100000, 2500000], smooth: true }
  ]
}))

const matrixOption = computed(() => ({
  tooltip: { position: 'top' },
  grid: { height: '70%', top: '10%' },
  xAxis: { type: 'category', data: ['Web', 'DB', 'Cache', 'MQ', 'API'], splitArea: { show: true } },
  yAxis: { type: 'category', data: ['Web', 'DB', 'Cache', 'MQ', 'API'], splitArea: { show: true } },
  visualMap: { min: 0, max: 1000, calculable: true, orient: 'horizontal', left: 'center', bottom: '0%', inRange: { color: ['#e0f7fa', '#165DFF'] } },
  series: [{
    type: 'heatmap', data: [
      [0,0,0], [1,0,200], [2,0,50], [3,0,100], [4,0,300],
      [0,1,200], [1,1,0], [2,1,80], [3,1,150], [4,1,100],
      [0,2,50], [1,2,80], [2,2,0], [3,2,20], [4,2,60],
      [0,3,100], [1,3,150], [2,3,20], [3,3,0], [4,3,80],
      [0,4,300], [1,4,100], [2,4,60], [3,4,80], [4,4,0],
    ]
  }]
}))

const topologyOption = computed(() => ({
  tooltip: {},
  series: [{
    type: 'graph', layout: 'force', symbolSize: 50, roam: true,
    label: { show: true },
    edgeSymbol: ['circle', 'arrow'], edgeSymbolSize: [4, 10],
    data: [
      { name: 'Gateway', category: 0 }, { name: 'LB', category: 1 },
      { name: 'Web-1', category: 2 }, { name: 'Web-2', category: 2 },
      { name: 'DB-1', category: 3 }, { name: 'Cache', category: 3 },
    ],
    links: [
      { source: 'Gateway', target: 'LB' }, { source: 'LB', target: 'Web-1' },
      { source: 'LB', target: 'Web-2' }, { source: 'Web-1', target: 'DB-1' },
      { source: 'Web-2', target: 'DB-1' }, { source: 'Web-1', target: 'Cache' },
    ],
    categories: [{ name: '网关' }, { name: '负载均衡' }, { name: 'Web' }, { name: '数据' }],
    force: { repulsion: 1000, edgeLength: 120 }
  }]
}))

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

onMounted(fetchData)
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
      color: var(--el-text-color-primary);
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
      color: var(--el-text-color-primary);
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

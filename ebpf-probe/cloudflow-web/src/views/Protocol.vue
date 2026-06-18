<template>
  <div class="protocol">
    <div class="page-header">
      <h2 class="page-title">L7 协议分析</h2>
    </div>
    <el-tabs v-model="activeTab" type="border-card" class="protocol-tabs">
      <el-tab-pane label="HTTP" name="http">
        <el-row :gutter="16" class="metric-row">
          <el-col :span="6">
            <el-card :body-style="{ padding: '16px' }">
              <div class="metric-label">请求率</div>
              <div class="metric-num">{{ httpData.reqRate }}/s</div>
            </el-card>
          </el-col>
          <el-col :span="6">
            <el-card :body-style="{ padding: '16px' }">
              <div class="metric-label">2xx 率</div>
              <div class="metric-num" style="color: var(--el-color-success)">{{ httpData.okRate }}%</div>
            </el-card>
          </el-col>
          <el-col :span="6">
            <el-card :body-style="{ padding: '16px' }">
              <div class="metric-label">5xx 率</div>
              <div class="metric-num" style="color: var(--el-color-danger)">{{ httpData.errRate }}%</div>
            </el-card>
          </el-col>
          <el-col :span="6">
            <el-card :body-style="{ padding: '16px' }">
              <div class="metric-label">平均时延</div>
              <div class="metric-num">{{ httpData.avgLatency }}ms</div>
            </el-card>
          </el-col>
        </el-row>
        <el-card class="table-card" :body-style="{ padding: '20px' }">
          <div class="card-title">HTTP 请求日志</div>
          <el-table :data="httpData.logs" size="small" style="width: 100%">
            <el-table-column prop="timestamp" label="时间" width="160" />
            <el-table-column prop="clientIp" label="客户端IP" />
            <el-table-column prop="method" label="Method" width="80" />
            <el-table-column prop="url" label="URL" show-overflow-tooltip />
            <el-table-column prop="statusCode" label="状态码" width="80">
              <template #default="{ row }">
                <el-tag :type="row.statusCode >= 500 ? 'danger' : row.statusCode >= 400 ? 'warning' : 'success'" size="small">{{ row.statusCode }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="latency" label="时延(ms)" width="90" />
            <el-table-column prop="userAgent" label="User-Agent" show-overflow-tooltip />
          </el-table>
        </el-card>
      </el-tab-pane>
      <el-tab-pane label="DNS" name="dns">
        <el-row :gutter="16" class="metric-row">
          <el-col :span="6">
            <el-card :body-style="{ padding: '16px' }">
              <div class="metric-label">查询率</div>
              <div class="metric-num">{{ dnsData.queryRate }}/s</div>
            </el-card>
          </el-col>
          <el-col :span="6">
            <el-card :body-style="{ padding: '16px' }">
              <div class="metric-label">NXDOMAIN 率</div>
              <div class="metric-num" style="color: var(--el-color-warning)">{{ dnsData.nxdomainRate }}%</div>
            </el-card>
          </el-col>
          <el-col :span="6">
            <el-card :body-style="{ padding: '16px' }">
              <div class="metric-label">TOP 域名</div>
              <div class="metric-num" style="font-size: 14px">{{ dnsData.topDomain }}</div>
            </el-card>
          </el-col>
          <el-col :span="6">
            <el-card :body-style="{ padding: '16px' }">
              <div class="metric-label">平均时延</div>
              <div class="metric-num">{{ dnsData.avgLatency }}ms</div>
            </el-card>
          </el-col>
        </el-row>
        <el-card class="table-card" :body-style="{ padding: '20px' }">
          <div class="card-title">DNS 查询日志</div>
          <el-table :data="dnsData.logs" size="small" style="width: 100%">
            <el-table-column prop="timestamp" label="时间" width="160" />
            <el-table-column prop="client" label="客户端" />
            <el-table-column prop="domain" label="域名" show-overflow-tooltip />
            <el-table-column prop="qType" label="记录类型" width="90" />
            <el-table-column prop="response" label="响应" />
            <el-table-column prop="latency" label="时延(ms)" width="90" />
            <el-table-column prop="isNxdomain" label="异常" width="80">
              <template #default="{ row }">
                <el-tag v-if="row.isNxdomain" type="danger" size="small">NXDOMAIN</el-tag>
                <span v-else>-</span>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { getProtocolData } from '@/api/protocol'

const activeTab = ref('http')

const httpData = reactive({
  reqRate: 1240, okRate: 98.5, errRate: 0.3, avgLatency: 45,
  logs: [
    { timestamp: '2026-06-18 14:00:00', clientIp: '192.168.1.101', method: 'GET', url: '/api/v1/dashboard', statusCode: 200, latency: 32, userAgent: 'Mozilla/5.0' },
    { timestamp: '2026-06-18 14:00:01', clientIp: '192.168.1.102', method: 'POST', url: '/api/v1/login', statusCode: 200, latency: 56, userAgent: 'curl/7.68.0' },
    { timestamp: '2026-06-18 14:00:02', clientIp: '192.168.1.103', method: 'GET', url: '/api/v1/probes', statusCode: 500, latency: 1200, userAgent: 'Mozilla/5.0' },
  ]
})

const dnsData = reactive({
  queryRate: 580, nxdomainRate: 2.1, topDomain: 'example.com', avgLatency: 12,
  logs: [
    { timestamp: '2026-06-18 14:00:00', client: '192.168.1.101', domain: 'www.example.com', qType: 'A', response: '93.184.216.34', latency: 8, isNxdomain: false },
    { timestamp: '2026-06-18 14:00:01', client: '192.168.1.102', domain: 'api.internal.local', qType: 'A', response: '', latency: 15, isNxdomain: true },
    { timestamp: '2026-06-18 14:00:02', client: '192.168.1.103', domain: 'db-cluster.local', qType: 'A', response: '192.168.1.10', latency: 5, isNxdomain: false },
  ]
})

const fetchData = async () => {
  try {
    const res = await getProtocolData(activeTab.value as 'http' | 'dns', {})
    if (res.code === 0) {
      if (activeTab.value === 'http') Object.assign(httpData, res.data.http)
      else Object.assign(dnsData, res.data.dns)
    }
  } catch (e) { /* ignore */ }
}

onMounted(fetchData)
</script>

<style scoped lang="scss">
.protocol {
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
  }
  .protocol-tabs {
    border-radius: 8px;
  }
  .metric-row {
    margin-bottom: 16px;
    .metric-label {
      font-size: 12px;
      color: var(--el-text-color-secondary);
      margin-bottom: 4px;
    }
    .metric-num {
      font-size: 24px;
      font-weight: 600;
      color: var(--el-text-color-primary);
    }
  }
  .table-card {
    border-radius: 8px;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
    .card-title {
      font-size: 16px;
      font-weight: 600;
      color: var(--el-text-color-primary);
      margin-bottom: 16px;
    }
  }
}
</style>

<template>
  <div class="logs">
    <div class="page-header">
      <h2 class="page-title">协议日志</h2>
      <div class="header-actions">
        <el-radio-group v-model="logType" size="small" @change="fetchLogs">
          <el-radio-button value="http">HTTP</el-radio-button>
          <el-radio-button value="dns">DNS</el-radio-button>
        </el-radio-group>
        <el-input v-model="searchText" placeholder="搜索 URL/IP/域名" size="small" clearable style="width: 200px" @clear="fetchLogs" />
        <el-button type="primary" size="small" @click="fetchLogs">查询</el-button>
      </div>
    </div>

    <!-- HTTP统计 -->
    <template v-if="logType === 'http'">
      <el-row :gutter="16" class="stat-row">
        <el-col :span="6">
          <el-card class="stat-mini" :body-style="{ padding: '12px 16px' }">
            <div class="s-label">总请求数</div>
            <div class="s-value">{{ httpStats.totalRequests || logs.length }}</div>
          </el-card>
        </el-col>
        <el-col :span="6">
          <el-card class="stat-mini" :body-style="{ padding: '12px 16px' }">
            <div class="s-label">平均延迟</div>
            <div class="s-value">{{ (httpStats.avgLatency || 0).toFixed(1) }} ms</div>
          </el-card>
        </el-col>
        <el-col :span="6">
          <el-card class="stat-mini" :body-style="{ padding: '12px 16px' }">
            <div class="s-label">错误率</div>
            <div class="s-value" :class="(httpStats.errRate || 0) > 5 ? 'red' : ''">{{ (httpStats.errRate || 0).toFixed(1) }}%</div>
          </el-card>
        </el-col>
        <el-col :span="6">
          <el-card class="stat-mini" :body-style="{ padding: '12px 16px' }">
            <div class="s-label">数据源</div>
            <div class="s-value blue">eBPF</div>
          </el-card>
        </el-col>
      </el-row>

      <el-card class="table-card" :body-style="{ padding: '20px' }">
        <div class="card-title">HTTP 访问日志 <el-tag size="small" type="success" style="margin-left:8px">{{ logs.length }} 条</el-tag></div>
        <el-table :data="filteredLogs" size="small" style="width:100%" v-loading="loading" max-height="500">
          <el-table-column prop="timestamp" label="时间" width="165" />
          <el-table-column prop="clientIp" label="客户端IP" width="130" />
          <el-table-column prop="method" label="方法" width="70">
            <template #default="{ row }">
              <el-tag :type="methodColor(row.method)" size="small">{{ row.method }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="url" label="URL" show-overflow-tooltip />
          <el-table-column prop="statusCode" label="状态码" width="80">
            <template #default="{ row }">
              <el-tag v-if="row.statusCode" :type="row.statusCode >= 400 ? 'danger' : row.statusCode >= 300 ? 'warning' : 'success'" size="small">{{ row.statusCode }}</el-tag>
              <span v-else class="text-muted">-</span>
            </template>
          </el-table-column>
          <el-table-column prop="latency" label="延迟(ms)" width="90" />
          <el-table-column prop="userAgent" label="UA" width="120" show-overflow-tooltip />
        </el-table>
        <el-pagination class="pagination" background layout="total, prev, pager, next" :total="filteredLogs.length" :page-size="20" v-model:current-page="currentPage" />
      </el-card>
    </template>

    <!-- DNS日志 -->
    <template v-else-if="logType === 'dns'">
      <el-card class="table-card" :body-style="{ padding: '20px' }">
        <div class="card-title">DNS 查询日志 <el-tag size="small" type="success" style="margin-left:8px">{{ logs.length }} 条</el-tag></div>
        <el-table :data="filteredLogs" size="small" style="width:100%" v-loading="loading" max-height="600">
          <el-table-column prop="timestamp" label="时间" width="165" />
          <el-table-column prop="srcIp" label="源IP" width="130" />
          <el-table-column prop="domain" label="查询域名" show-overflow-tooltip />
          <el-table-column prop="queryType" label="类型" width="70" />
          <el-table-column prop="answer" label="结果" show-overflow-tooltip />
          <el-table-column prop="latency" label="延迟(ms)" width="90" />
        </el-table>
        <el-pagination class="pagination" background layout="total, prev, pager, next" :total="filteredLogs.length" :page-size="20" v-model:current-page="currentPage" />
      </el-card>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import request from '@/utils/request'

const logType = ref('http')
const searchText = ref('')
const loading = ref(false)
const currentPage = ref(1)
const logs = ref<any[]>([])
const httpStats = ref<{ totalRequests?: number; avgLatency?: number; errRate?: number }>({})

const methodColor = (m: string) => {
  const map: Record<string, string> = { GET: 'success', POST: 'primary', PUT: 'warning', DELETE: 'danger' }
  return map[m] || ''
}

const filteredLogs = computed(() => {
  let list = logs.value
  if (searchText.value) {
    const f = searchText.value.toLowerCase()
    list = list.filter(l =>
      (l.url || '').toLowerCase().includes(f) ||
      (l.clientIp || '').toLowerCase().includes(f) ||
      (l.domain || '').toLowerCase().includes(f) ||
      (l.srcIp || '').toLowerCase().includes(f)
    )
  }
  const start = (currentPage.value - 1) * 20
  return list.slice(start, start + 20)
})

const fetchLogs = async () => {
  loading.value = true
  try {
    if (logType.value === 'http') {
      const res = await request.get('/protocol/http') as any
      if (res.code === 0 && res.data) {
        logs.value = res.data.logs || []
        httpStats.value = {
          totalRequests: res.data.totalRequests,
          avgLatency: res.data.avgLatency,
          errRate: res.data.errRate
        }
      }
    } else if (logType.value === 'dns') {
      const res = await request.get('/protocol/dns') as any
      if (res.code === 0 && Array.isArray(res.data)) logs.value = res.data
      else if (res.code === 0 && res.data && res.data.logs) logs.value = res.data.logs
    }
  } catch (e) { console.error('Logs fetch error:', e) } finally { loading.value = false }
}

onMounted(fetchLogs)
</script>

<style scoped lang="scss">
.logs {
  .page-header {
    display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;
    .page-title { font-size: 18px; font-weight: 600; color: var(--el-text-color-primary); }
    .header-actions { display: flex; gap: 12px; align-items: center; }
  }
  .stat-row { margin-bottom: 16px; }
  .stat-mini {
    border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.08); text-align: center;
    .s-label { font-size: 12px; color: var(--el-text-color-secondary); }
    .s-value { font-size: 22px; font-weight: 700; color: var(--el-text-color-primary); margin-top: 4px;
      &.red { color: #F56C6C; }
      &.blue { color: #409EFF; }
    }
  }
  .table-card {
    border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.08);
    .card-title { font-size: 14px; font-weight: 600; color: var(--el-text-color-primary); margin-bottom: 12px; display: flex; align-items: center; }
    .pagination { margin-top: 16px; justify-content: flex-end; }
  }
  .text-muted { color: var(--el-text-color-placeholder); }
}
</style>

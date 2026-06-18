<template>
  <div class="logs">
    <div class="page-header">
      <h2 class="page-title">日志查询</h2>
      <div class="header-actions">
        <el-select v-model="logType" placeholder="日志类型" size="small" style="width: 140px">
          <el-option label="系统日志" value="system" />
          <el-option label="网络日志" value="network" />
          <el-option label="安全日志" value="security" />
          <el-option label="应用日志" value="application" />
        </el-select>
        <el-input v-model="searchText" placeholder="关键词搜索" size="small" clearable style="width: 200px" />
        <el-button type="primary" size="small" @click="fetchLogs">查询</el-button>
      </div>
    </div>
    <el-card class="table-card" :body-style="{ padding: '20px' }">
      <el-table :data="logs" size="small" style="width:100%">
        <el-table-column prop="timestamp" label="时间" width="160" />
        <el-table-column prop="level" label="级别" width="80">
          <template #default="{ row }">
            <el-tag :type="row.level === 'error' ? 'danger' : row.level === 'warn' ? 'warning' : 'info'" size="small">{{ row.level }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="source" label="来源" width="120" />
        <el-table-column prop="host" label="主机" width="120" />
        <el-table-column prop="message" label="内容" show-overflow-tooltip />
      </el-table>
      <el-pagination class="pagination" background layout="total, prev, pager, next" :total="logs.length" :page-size="20" />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const logType = ref('system')
const searchText = ref('')

const logs = ref([
  { timestamp: '2026-06-18 14:00:00', level: 'info', source: 'kernel', host: 'node-01', message: 'ebpf probe loaded successfully' },
  { timestamp: '2026-06-18 14:00:01', level: 'info', source: 'network', host: 'node-01', message: 'TCP connection established: 192.168.1.101:443 -> 192.168.1.102:54321' },
  { timestamp: '2026-06-18 14:00:02', level: 'warn', source: 'security', host: 'node-02', message: 'Suspicious outbound connection detected to 8.8.8.8:53' },
  { timestamp: '2026-06-18 14:00:03', level: 'error', source: 'application', host: 'node-03', message: 'HTTP request timeout: /api/v1/probes' },
  { timestamp: '2026-06-18 14:00:04', level: 'info', source: 'network', host: 'node-01', message: 'DNS query: www.example.com -> 93.184.216.34' },
  { timestamp: '2026-06-18 14:00:05', level: 'warn', source: 'system', host: 'node-02', message: 'High CPU usage detected: 85%' },
])

const fetchLogs = () => {
  // TODO: call API
}
</script>

<style scoped lang="scss">
.logs {
  .page-header {
    display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;
    .page-title { font-size: 18px; font-weight: 600; color: #FFFFFF; }
    .header-actions { display: flex; gap: 12px; align-items: center; }
  }
  .table-card {
    border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.08);
    .pagination { margin-top: 16px; justify-content: flex-end; }
  }
}
</style>

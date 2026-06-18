<template>
  <div class="alerts">
    <div class="page-header">
      <h2 class="page-title">告警中心</h2>
      <div class="header-actions">
        <el-select v-model="filterLevel" placeholder="级别" size="small" clearable style="width: 120px">
          <el-option label="全部" value="" />
          <el-option label="高危" value="high" />
          <el-option label="中危" value="medium" />
          <el-option label="低危" value="low" />
        </el-select>
        <el-select v-model="filterStatus" placeholder="状态" size="small" clearable style="width: 120px">
          <el-option label="全部" value="" />
          <el-option label="新建" value="new" />
          <el-option label="已处理" value="handled" />
          <el-option label="已忽略" value="ignored" />
        </el-select>
        <el-button type="primary" size="small" @click="fetchEvents">刷新</el-button>
      </div>
    </div>

    <!-- 统计卡 -->
    <el-row :gutter="16" class="stat-row">
      <el-col :span="6">
        <el-card class="stat-mini" :body-style="{ padding: '12px 16px' }">
          <div class="s-label">总告警数</div>
          <div class="s-value">{{ events.length }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="stat-mini danger" :body-style="{ padding: '12px 16px' }">
          <div class="s-label">高危</div>
          <div class="s-value red">{{ countByLevel('high') }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="stat-mini warning" :body-style="{ padding: '12px 16px' }">
          <div class="s-label">中危</div>
          <div class="s-value orange">{{ countByLevel('medium') }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="stat-mini" :body-style="{ padding: '12px 16px' }">
          <div class="s-label">低危</div>
          <div class="s-value">{{ countByLevel('low') }}</div>
        </el-card>
      </el-col>
    </el-row>

    <el-card class="table-card" :body-style="{ padding: '20px' }">
      <el-table :data="filteredEvents" size="small" style="width:100%" v-loading="loading">
        <el-table-column prop="time" label="时间" width="165" />
        <el-table-column prop="severity" label="级别" width="80">
          <template #default="{ row }">
            <el-tag :type="severityColor(row.severity)" size="small">{{ row.severity }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="type" label="类型" width="120" />
        <el-table-column prop="description" label="描述" show-overflow-tooltip />
        <el-table-column label="源/目的" width="200">
          <template #default="{ row }">
            <span v-if="row.src || row.dst">{{ row.src }} → {{ row.dst }}</span>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="statusColor(row.status)" size="small">{{ statusText(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button v-if="row.status === 'new'" type="success" link size="small" @click="handleEvent(row, 'handled')">处理</el-button>
            <el-button v-if="row.status === 'new'" type="info" link size="small" @click="handleEvent(row, 'ignored')">忽略</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-pagination
        class="pagination" background layout="total, prev, pager, next"
        :total="filteredEvents.length" :page-size="pageSize" v-model:current-page="currentPage"
      />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import request from '@/utils/request'

const loading = ref(false)
const filterLevel = ref('')
const filterStatus = ref('')
const currentPage = ref(1)
const pageSize = 20

interface SecurityEvent {
  time: string
  severity: string
  type: string
  description: string
  src: string
  dst: string
  status: string
}

const events = ref<SecurityEvent[]>([])

const severityColor = (s: string) => {
  const m: Record<string, string> = { high: 'danger', medium: 'warning', low: 'info', critical: 'danger' }
  return m[s] || ''
}
const statusColor = (s: string) => {
  const m: Record<string, string> = { new: 'danger', handled: 'success', ignored: 'info' }
  return m[s] || ''
}
const statusText = (s: string) => {
  const m: Record<string, string> = { new: '新建', handled: '已处理', ignored: '已忽略' }
  return m[s] || s
}
const countByLevel = (lvl: string) => events.value.filter(e => e.severity === lvl).length

const filteredEvents = computed(() => {
  let list = events.value
  if (filterLevel.value) list = list.filter(e => e.severity === filterLevel.value)
  if (filterStatus.value) list = list.filter(e => e.status === filterStatus.value)
  const start = (currentPage.value - 1) * pageSize
  return list.slice(start, start + pageSize)
})

const handleEvent = (event: SecurityEvent, status: string) => {
  event.status = status
  ElMessage.success('状态已更新')
}

const fetchEvents = async () => {
  loading.value = true
  try {
    const res = await request.get('/security/events') as any
    if (res.code === 0 && Array.isArray(res.data)) {
      events.value = res.data
    }
  } catch (e) { console.error('Alerts fetch error:', e) } finally { loading.value = false }
}

let timer: ReturnType<typeof setInterval> | null = null
onMounted(() => { fetchEvents(); timer = setInterval(fetchEvents, 30000) })
onUnmounted(() => { if (timer) clearInterval(timer) })
</script>

<style scoped lang="scss">
.alerts {
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
      &.orange { color: #E6A23C; }
    }
  }
  .table-card {
    border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.08);
    .pagination { margin-top: 16px; justify-content: flex-end; }
  }
  .text-muted { color: var(--el-text-color-placeholder); }
}
</style>

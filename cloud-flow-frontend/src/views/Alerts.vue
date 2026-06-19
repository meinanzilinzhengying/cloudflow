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
          <el-option label="未处理" value="pending" />
          <el-option label="已处理" value="handled" />
          <el-option label="已忽略" value="ignored" />
        </el-select>
        <el-button type="primary" size="small" @click="fetchEvents">刷新</el-button>
      </div>
    </div>
    <el-card class="table-card" :body-style="{ padding: '20px' }">
      <el-table :data="filteredEvents" size="small" style="width:100%">
        <el-table-column prop="timestamp" label="时间" width="160" />
        <el-table-column prop="level" label="级别" width="80">
          <template #default="{ row }">
            <el-tag :type="row.level === 'high' ? 'danger' : row.level === 'medium' ? 'warning' : 'info'" size="small">{{ row.level }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="type" label="类型" width="120" />
        <el-table-column prop="description" label="描述" show-overflow-tooltip />
        <el-table-column prop="host" label="主机" width="120" />
        <el-table-column prop="status" label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="statusColor(row.status)" size="small">{{ statusText(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180">
          <template #default="{ row }">
            <el-button v-if="row.status === 'pending'" type="success" link size="small" @click="handleEvent(row.id, 'handled')">已处理</el-button>
            <el-button v-if="row.status === 'pending'" type="info" link size="small" @click="handleEvent(row.id, 'ignored')">忽略</el-button>
            <el-button type="primary" link size="small" @click="showDetail(row)">详情</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="detailVisible" title="告警详情" width="600px">
      <el-descriptions :column="1" border v-if="currentEvent">
        <el-descriptions-item label="事件ID">{{ currentEvent.id }}</el-descriptions-item>
        <el-descriptions-item label="时间">{{ currentEvent.timestamp }}</el-descriptions-item>
        <el-descriptions-item label="级别">
          <el-tag :type="currentEvent.level === 'high' ? 'danger' : currentEvent.level === 'medium' ? 'warning' : 'info'" size="small">{{ currentEvent.level }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="类型">{{ currentEvent.type }}</el-descriptions-item>
        <el-descriptions-item label="主机">{{ currentEvent.host }}</el-descriptions-item>
        <el-descriptions-item label="描述">{{ currentEvent.description }}</el-descriptions-item>
        <el-descriptions-item label="原始数据">
          <el-input type="textarea" :rows="4" :model-value="currentEvent.rawData" readonly />
        </el-descriptions-item>
      </el-descriptions>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { ElMessage } from 'element-plus'

const filterLevel = ref('')
const filterStatus = ref('')
const detailVisible = ref(false)
const currentEvent = ref<any>(null)

const events = ref([
  { id: 'S001', timestamp: '2026-06-18 14:00:00', level: 'high', status: 'pending', type: '权限提升', description: '检测到进程尝试获取 CAP_SYS_ADMIN 权限', host: 'node-01', rawData: '{"pid":1234,"comm":"python3","cap":21}' },
  { id: 'S002', timestamp: '2026-06-18 13:45:00', level: 'medium', status: 'pending', type: '异常连接', description: '发现非常规端口的外连行为', host: 'node-02', rawData: '{"src":"192.168.1.102:54321","dst":"8.8.8.8:53"}' },
  { id: 'S003', timestamp: '2026-06-18 13:30:00', level: 'low', status: 'handled', type: '文件访问', description: '敏感文件 /etc/shadow 被读取', host: 'node-01', rawData: '{"pid":5678,"filename":"/etc/shadow","uid":0}' },
  { id: 'S004', timestamp: '2026-06-18 13:00:00', level: 'high', status: 'ignored', type: '模块加载', description: '检测到未签名内核模块加载', host: 'node-03', rawData: '{"module":"unknown.ko","pid":9999}' },
  { id: 'S005', timestamp: '2026-06-18 12:30:00', level: 'medium', status: 'pending', type: 'DNS隧道', description: '检测到可疑DNS查询长度，疑似DNS隧道', host: 'node-02', rawData: '{"domain":"a.b.c.d.e.f.g.h.i.j.k.l.m.n.o.p.q.r.s.t.u.v.w.x.y.z.example.com","len":120}' },
])

const filteredEvents = computed(() => {
  let list = events.value
  if (filterLevel.value) list = list.filter(e => e.level === filterLevel.value)
  if (filterStatus.value) list = list.filter(e => e.status === filterStatus.value)
  return list
})

const statusColor = (status: string) => {
  const map: Record<string, string> = { pending: 'danger', handled: 'success', ignored: 'info' }
  return map[status] || ''
}
const statusText = (status: string) => {
  const map: Record<string, string> = { pending: '未处理', handled: '已处理', ignored: '已忽略' }
  return map[status] || status
}

const handleEvent = (id: string, status: string) => {
  const ev = events.value.find(e => e.id === id)
  if (ev) ev.status = status
  ElMessage.success('状态已更新')
}

const showDetail = (event: any) => {
  currentEvent.value = event
  detailVisible.value = true
}

const fetchEvents = () => {
  // TODO: call API
}
</script>

<style scoped lang="scss">
.alerts {
  .page-header {
    display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;
    .page-title { font-size: 18px; font-weight: 600; color: #FFFFFF; }
    .header-actions { display: flex; gap: 12px; align-items: center; }
  }
  .table-card {
    background: rgba(10, 186, 255, 0.08);
    border: 1px solid rgba(10, 186, 255, 0.3);
    border-radius: 8px;
    box-shadow: 0 2px 8px rgba(0,0,0,0.08);
  }
}
</style>

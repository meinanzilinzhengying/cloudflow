<template>
  <div class="security">
    <div class="page-header">
      <h2 class="page-title">安全审计</h2>
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
    <el-timeline class="event-timeline">
      <el-timeline-item
        v-for="event in filteredEvents"
        :key="event.id"
        :type="event.level === 'high' ? 'danger' : event.level === 'medium' ? 'warning' : 'info'"
        :icon="event.level === 'high' ? WarningFilled : event.level === 'medium' ? Warning : InfoFilled"
        :timestamp="event.timestamp"
      >
        <el-card
          class="event-card"
          :body-style="{ padding: '16px' }"
          :class="{ 'high-risk': event.level === 'high', 'medium-risk': event.level === 'medium', 'low-risk': event.level === 'low' }"
        >
          <div class="event-header">
            <div class="event-title">
              <StatusTag :status="event.level" />
              <span class="event-type">{{ event.type }}</span>
            </div>
            <div class="event-actions">
              <el-button v-if="event.status === 'pending'" type="success" link size="small" @click="handleEvent(event.id, 'handled')">标记已处理</el-button>
              <el-button v-if="event.status === 'pending'" type="info" link size="small" @click="handleEvent(event.id, 'ignored')">忽略</el-button>
              <el-button type="primary" link size="small" @click="showDetail(event)">详情</el-button>
            </div>
          </div>
          <p class="event-desc">{{ event.description }}</p>
          <div class="event-meta">
            <el-tag size="small" type="info">{{ event.host }}</el-tag>
            <el-tag size="small" :type="statusColor(event.status)">{{ statusText(event.status) }}</el-tag>
          </div>
        </el-card>
      </el-timeline-item>
    </el-timeline>

    <el-dialog v-model="detailVisible" title="事件详情" width="600px">
      <el-descriptions :column="1" border v-if="currentEvent">
        <el-descriptions-item label="事件ID">{{ currentEvent.id }}</el-descriptions-item>
        <el-descriptions-item label="时间">{{ currentEvent.timestamp }}</el-descriptions-item>
        <el-descriptions-item label="级别">
          <StatusTag :status="currentEvent.level" />
        </el-descriptions-item>
        <el-descriptions-item label="类型">{{ currentEvent.type }}</el-descriptions-item>
        <el-descriptions-item label="主机">{{ currentEvent.host }}</el-descriptions-item>
        <el-descriptions-item label="描述">{{ currentEvent.description }}</el-descriptions-item>
        <el-descriptions-item label="原始数据">
          <el-input type="textarea" :rows="4" :model-value="currentEvent.rawData" readonly />
        </el-descriptions-item>
        <el-descriptions-item label="建议处理方案">{{ currentEvent.suggestion }}</el-descriptions-item>
      </el-descriptions>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { WarningFilled, Warning, InfoFilled } from '@element-plus/icons-vue'
import StatusTag from '@/components/StatusTag.vue'
import { getSecurityEvents, updateEventStatus } from '@/api/security'
import type { SecurityEvent } from '@/api/security'
import { statusColor, statusText } from '@/utils/format'

const filterLevel = ref('')
const filterStatus = ref('')
const events = ref<SecurityEvent[]>([])
const detailVisible = ref(false)
const currentEvent = ref<SecurityEvent | null>(null)

const filteredEvents = computed(() => {
  let list = events.value
  if (filterLevel.value) list = list.filter(e => e.level === filterLevel.value)
  if (filterStatus.value) list = list.filter(e => e.status === filterStatus.value)
  return list
})

const fetchEvents = async () => {
  try {
    const res = await getSecurityEvents({})
    if (res.code === 0) events.value = res.data
    else {
      events.value = [
        { id: 'S001', timestamp: '2026-06-18 14:00:00', level: 'high', status: 'pending', type: '权限提升', description: '检测到进程尝试获取 CAP_SYS_ADMIN 权限', host: 'node-01', rawData: '{"pid":1234,"comm":"python3","cap":21}', suggestion: '审查该进程合法性，必要时kill进程' },
        { id: 'S002', timestamp: '2026-06-18 13:45:00', level: 'medium', status: 'pending', type: '异常连接', description: '发现非常规端口的外连行为', host: 'node-02', rawData: '{"src":"192.168.1.102:54321","dst":"8.8.8.8:53"}', suggestion: '检查是否为DNS查询，或存在数据泄露风险' },
        { id: 'S003', timestamp: '2026-06-18 13:30:00', level: 'low', status: 'handled', type: '文件访问', description: '敏感文件 /etc/shadow 被读取', host: 'node-01', rawData: '{"pid":5678,"filename":"/etc/shadow","uid":0}', suggestion: '确认是否为正常运维操作' },
        { id: 'S004', timestamp: '2026-06-18 13:00:00', level: 'high', status: 'ignored', type: '模块加载', description: '检测到未签名内核模块加载', host: 'node-03', rawData: '{"module":"unknown.ko","pid":9999}', suggestion: '立即卸载模块并检查系统完整性' },
        { id: 'S005', timestamp: '2026-06-18 12:30:00', level: 'medium', status: 'pending', type: 'DNS隧道', description: '检测到可疑DNS查询长度，疑似DNS隧道', host: 'node-02', rawData: '{"domain":"a.b.c.d.e.f.g.h.i.j.k.l.m.n.o.p.q.r.s.t.u.v.w.x.y.z.example.com","len":120}', suggestion: '阻断该域名并分析后续流量' },
      ]
    }
  } catch (e) { /* ignore */ }
}

const handleEvent = async (id: string, status: string) => {
  try {
    await updateEventStatus(id, status)
    ElMessage.success('状态已更新')
    fetchEvents()
  } catch (e) {
    ElMessage.error('更新失败')
  }
}

const showDetail = (event: SecurityEvent) => {
  currentEvent.value = event
  detailVisible.value = true
}

onMounted(fetchEvents)
</script>

<style scoped lang="scss">
.security {
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
  .event-timeline {
    padding-left: 20px;
  }
  .event-card {
    border-radius: 8px;
    &.high-risk { border-left: 4px solid var(--el-color-danger); }
    &.medium-risk { border-left: 4px solid var(--el-color-warning); }
    &.low-risk { border-left: 4px solid var(--el-color-info); }
    .event-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: 8px;
      .event-title {
        display: flex;
        align-items: center;
        gap: 8px;
        .event-type {
          font-weight: 600;
          color: var(--el-text-color-primary);
        }
      }
      .event-actions {
        display: flex;
        gap: 8px;
      }
    }
    .event-desc {
      font-size: 14px;
      color: var(--el-text-color-regular);
      margin-bottom: 8px;
    }
    .event-meta {
      display: flex;
      gap: 8px;
    }
  }
}
</style>

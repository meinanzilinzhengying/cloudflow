<template>
  <div class="probes">
    <div class="page-header">
      <h2 class="page-title">探针管理</h2>
      <div class="header-actions">
        <el-radio-group v-model="filterStatus" size="small">
          <el-radio-button label="all">全部</el-radio-button>
          <el-radio-button label="online">在线</el-radio-button>
          <el-radio-button label="offline">离线</el-radio-button>
          <el-radio-button label="degraded">异常</el-radio-button>
        </el-radio-group>
        <el-select v-model="filterVersion" placeholder="版本" size="small" clearable style="width: 120px">
          <el-option label="v3.1.0" value="v3.1.0" />
          <el-option label="v3.0.0" value="v3.0.0" />
        </el-select>
        <el-button type="primary" size="small" :icon="Refresh">刷新</el-button>
      </div>
    </div>
    <el-card class="table-card" :body-style="{ padding: '20px' }">
      <el-table :data="filteredProbes" size="small" style="width: 100%" v-loading="loading">
        <el-table-column prop="id" label="探针ID" width="100" sortable />
        <el-table-column prop="hostname" label="主机名" />
        <el-table-column prop="ip" label="IP地址" />
        <el-table-column prop="version" label="版本" width="100" />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <StatusTag :status="row.status" />
          </template>
        </el-table-column>
        <el-table-column prop="cpu" label="CPU" width="100">
          <template #default="{ row }">
            <el-progress :percentage="row.cpu" :status="row.cpu > 80 ? 'exception' : row.cpu > 50 ? 'warning' : 'success'" :stroke-width="8" />
          </template>
        </el-table-column>
        <el-table-column prop="memory" label="内存" width="100">
          <template #default="{ row }">
            {{ formatBytes(row.memory) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button type="primary" link size="small" @click="showDetail(row)">详情</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-pagination class="pagination" background layout="total, prev, pager, next" :total="filteredProbes.length" :page-size="10" />
    </el-card>

    <el-dialog v-model="detailVisible" title="探针详情" width="700px" destroy-on-close>
      <el-tabs v-model="activeTab">
        <el-tab-pane label="基本信息" name="basic">
          <el-descriptions :column="2" border>
            <el-descriptions-item label="探针ID">{{ currentProbe?.id }}</el-descriptions-item>
            <el-descriptions-item label="主机名">{{ currentProbe?.hostname }}</el-descriptions-item>
            <el-descriptions-item label="IP地址">{{ currentProbe?.ip }}</el-descriptions-item>
            <el-descriptions-item label="版本">{{ currentProbe?.version }}</el-descriptions-item>
            <el-descriptions-item label="内核">{{ currentProbe?.kernel }}</el-descriptions-item>
            <el-descriptions-item label="BTF">{{ currentProbe?.btf ? '支持' : '不支持' }}</el-descriptions-item>
            <el-descriptions-item label="运行时长">{{ currentProbe?.uptime }}s</el-descriptions-item>
          </el-descriptions>
        </el-tab-pane>
        <el-tab-pane label="资源监控" name="resource">
          <v-chart :option="resourceOption" autoresize style="height: 250px" />
        </el-tab-pane>
        <el-tab-pane label="采集状态" name="collectors">
          <el-table :data="collectorList" size="small">
            <el-table-column prop="name" label="采集器" />
            <el-table-column prop="status" label="状态">
              <template #default="{ row }">
                <el-tag :type="row.status ? 'success' : 'info'" size="small">{{ row.status ? '运行中' : '已关闭' }}</el-tag>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
        <el-tab-pane label="配置管理" name="config">
          <el-input v-model="configYaml" type="textarea" :rows="12" />
          <el-button type="primary" class="config-btn" @click="saveConfig">保存配置</el-button>
        </el-tab-pane>
      </el-tabs>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import StatusTag from '@/components/StatusTag.vue'
import { getProbes, updateProbeConfig } from '@/api/probe'
import type { Probe, ProbeDetail } from '@/api/probe'
import { formatBytes } from '@/utils/format'

const loading = ref(false)
const probes = ref<Probe[]>([])
const filterStatus = ref('all')
const filterVersion = ref('')
const detailVisible = ref(false)
const activeTab = ref('basic')
const currentProbe = ref<ProbeDetail | null>(null)
const configYaml = ref('')

const collectorList = ref([
  { name: 'network_flow', status: true }, { name: 'tcp_connect', status: true },
  { name: 'process_exec', status: true }, { name: 'file_open', status: true },
  { name: 'syscall', status: false }, { name: 'http_trace', status: false },
  { name: 'dns_trace', status: false }, { name: 'db_trace', status: false },
  { name: 'sched_trace', status: false }, { name: 'mem_trace', status: false },
  { name: 'block_trace', status: false }, { name: 'security_trace', status: false },
  { name: 'host_metrics', status: true },
])

const filteredProbes = computed(() => {
  let list = probes.value
  if (filterStatus.value !== 'all') list = list.filter(p => p.status === filterStatus.value)
  if (filterVersion.value) list = list.filter(p => p.version === filterVersion.value)
  return list
})

const resourceOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  legend: { data: ['CPU', '内存'] },
  xAxis: { type: 'category', data: ['00:00', '04:00', '08:00', '12:00', '16:00', '20:00'] },
  yAxis: { type: 'value' },
  series: [
    { name: 'CPU', type: 'line', data: [10, 15, 35, 45, 30, 20], smooth: true },
    { name: '内存', type: 'line', data: [20, 22, 30, 40, 35, 28], smooth: true }
  ]
}))

const fetchProbes = async () => {
  loading.value = true
  try {
    const res = await getProbes()
    if (res.code === 0) probes.value = res.data
    else {
      probes.value = [
        { id: 'P001', hostname: 'node-01', ip: '192.168.1.101', version: 'v3.1.0', status: 'online', cpu: 32, memory: 104857600, kernel: '5.14.0', btf: true, uptime: 86400, collectors: [] },
        { id: 'P002', hostname: 'node-02', ip: '192.168.1.102', version: 'v3.1.0', status: 'online', cpu: 45, memory: 157286400, kernel: '5.14.0', btf: true, uptime: 86400, collectors: [] },
        { id: 'P003', hostname: 'node-03', ip: '192.168.1.103', version: 'v3.0.0', status: 'degraded', cpu: 78, memory: 209715200, kernel: '5.4.0', btf: false, uptime: 3600, collectors: [] },
        { id: 'P004', hostname: 'node-04', ip: '192.168.1.104', version: 'v3.1.0', status: 'offline', cpu: 0, memory: 0, kernel: '5.14.0', btf: true, uptime: 0, collectors: [] },
      ]
    }
  } finally { loading.value = false }
}

const showDetail = async (row: Probe) => {
  currentProbe.value = row as ProbeDetail
  configYaml.value = `probe:\n  id: "${row.id}"\n  log_level: "info"\n  \ncollector:\n  network_flow: true\n  tcp_connect: true\n  process_exec: true\n  host_metrics: true\n  \noutput:\n  type: "clickhouse"\n  clickhouse:\n    addr: "192.168.58.130:9000"\n`
  detailVisible.value = true
}

const saveConfig = async () => {
  if (!currentProbe.value) return
  try {
    await updateProbeConfig(currentProbe.value.id, { config: configYaml.value })
    ElMessage.success('配置已保存')
  } catch (e) {
    ElMessage.error('保存失败')
  }
}

onMounted(fetchProbes)
</script>

<style scoped lang="scss">
.probes {
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
  .table-card {
    border-radius: 8px;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
    .pagination {
      margin-top: 16px;
      justify-content: flex-end;
    }
  }
  .config-btn {
    margin-top: 12px;
  }
}
</style>

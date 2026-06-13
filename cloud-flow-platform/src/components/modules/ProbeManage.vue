<template>
  <div>
    <!-- 探针统计 -->
    <div class="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
      <div class="bg-dark-800 rounded-xl p-4 border border-dark-600">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-gray-400 text-sm">探针总数</p>
            <p class="text-2xl font-bold text-white">{{ probes.length }}</p>
          </div>
          <Server class="w-8 h-8 text-gray-600" />
        </div>
      </div>
      <div class="bg-dark-800 rounded-xl p-4 border border-dark-600">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-gray-400 text-sm">在线探针</p>
            <p class="text-2xl font-bold text-green-400">{{ onlineCount }}</p>
          </div>
          <CheckCircle class="w-8 h-8 text-green-400" />
        </div>
      </div>
      <div class="bg-dark-800 rounded-xl p-4 border border-dark-600">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-gray-400 text-sm">离线探针</p>
            <p class="text-2xl font-bold text-red-400">{{ offlineCount }}</p>
          </div>
          <XCircle class="w-8 h-8 text-red-400" />
        </div>
      </div>
      <div class="bg-dark-800 rounded-xl p-4 border border-dark-600">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-gray-400 text-sm">异常探针</p>
            <p class="text-2xl font-bold text-amber-400">{{ warningCount }}</p>
          </div>
          <AlertCircle class="w-8 h-8 text-amber-400" />
        </div>
      </div>
    </div>

    <!-- 操作栏 -->
    <div class="flex flex-wrap items-center gap-4 mb-6">
      <button @click="showInstallModal = true" class="btn btn-primary">
        <Download class="w-4 h-4" />
        安装探针
      </button>
      <button @click="showSSHModal = true" class="btn btn-secondary">
        <Terminal class="w-4 h-4" />
        SSH 安装
      </button>
      <button @click="showK8sModal = true" class="btn btn-secondary">
        <FolderTree class="w-4 h-4" />
        K8s 部署
      </button>
      <div class="flex-1" />
      <select v-model="selectedType" class="select-dark">
        <option value="all">全部类型</option>
        <option value="agent">Agent</option>
        <option value="sidecar">Sidecar</option>
      </select>
      <select v-model="selectedGroup" class="select-dark">
        <option value="all">全部分组</option>
        <option v-for="g in groups" :key="g" :value="g">{{ g }}</option>
      </select>
      <button @click="fetchProbes" class="btn btn-ghost">
        <ArrowUpCircle class="w-4 h-4" />
        刷新
      </button>
    </div>

    <!-- 探针列表 -->
    <div class="card p-6">
      <div class="flex items-center justify-between mb-4">
        <h3 class="text-lg font-semibold text-white">探针列表</h3>
        <span class="text-sm text-gray-400">{{ filteredProbes.length }} 个探针</span>
      </div>

      <!-- Loading -->
      <div v-if="loading" class="flex items-center justify-center py-12">
        <Loader2 class="w-6 h-6 animate-spin text-primary-500" />
        <span class="ml-2 text-gray-400">加载中...</span>
      </div>

      <!-- Empty -->
      <div v-else-if="filteredProbes.length === 0" class="text-center py-12 text-gray-500">
        <Server class="w-12 h-12 mx-auto mb-3 opacity-30" />
        <p>暂无探针，请点击"安装探针"添加</p>
      </div>

      <!-- Table -->
      <div v-else class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="text-left text-gray-400 border-b border-dark-600">
              <th class="py-2 px-3">名称</th>
              <th class="py-2 px-3">类型</th>
              <th class="py-2 px-3">分组</th>
              <th class="py-2 px-3">IP/主机</th>
              <th class="py-2 px-3">状态</th>
              <th class="py-2 px-3">版本</th>
              <th class="py-2 px-3">最后心跳</th>
              <th class="py-2 px-3">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="probe in filteredProbes"
              :key="probe.id"
              class="border-b border-dark-700 hover:bg-dark-700/50"
            >
              <td class="py-3 px-3 text-white">{{ probe.name }}</td>
              <td class="py-3 px-3">
                <span class="badge" :class="probe.type === 'agent' ? 'badge-blue' : 'badge-purple'">
                  {{ probe.type === 'agent' ? 'Agent' : 'Sidecar' }}
                </span>
              </td>
              <td class="py-3 px-3 text-gray-300">{{ probe.group || '-' }}</td>
              <td class="py-3 px-3 text-gray-300 font-mono text-xs">{{ probe.ip || probe.hostname || '-' }}</td>
              <td class="py-3 px-3">
                <span class="flex items-center gap-1">
                  <span
                    class="w-2 h-2 rounded-full"
                    :class="probe.status === 'online' ? 'bg-green-500' : 'bg-red-500'"
                  ></span>
                  {{ probe.status === 'online' ? '在线' : '离线' }}
                </span>
              </td>
              <td class="py-3 px-3 text-gray-400">{{ probe.version || '-' }}</td>
              <td class="py-3 px-3 text-gray-400">{{ probe.lastHeartbeat || '-' }}</td>
              <td class="py-3 px-3">
                <div class="flex items-center gap-2">
                  <button @click="upgradeProbe(probe)" class="btn-icon text-blue-400" title="升级">
                    <ArrowUpCircle class="w-4 h-4" />
                  </button>
                  <button @click="uninstallProbe(probe)" class="btn-icon text-red-400" title="卸载">
                    <Trash2 class="w-4 h-4" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 安装探针弹窗（本地） -->
    <div v-if="showInstallModal" class="modal-overlay" @click.self="showInstallModal = false">
      <div class="modal">
        <div class="modal-header">
          <h3>安装探针（本地）</h3>
          <button @click="showInstallModal = false" class="btn-icon"><X class="w-5 h-5" /></button>
        </div>
        <div class="modal-body space-y-4">
          <div>
            <label class="label">探针名称</label>
            <input v-model="newProbe.name" class="input-dark" placeholder="请输入探针名称" />
          </div>
          <div>
            <label class="label">探针类型</label>
            <select v-model="newProbe.type" class="input-dark">
              <option value="agent">Agent</option>
              <option value="sidecar">Sidecar</option>
            </select>
          </div>
          <div>
            <label class="label">分组</label>
            <input v-model="newProbe.group" class="input-dark" placeholder="请输入分组名称" />
          </div>
        </div>
        <div class="modal-footer">
          <button @click="showInstallModal = false" class="btn btn-secondary">取消</button>
          <button @click="installProbe()" :disabled="!newProbe.name" class="btn btn-primary">安装</button>
        </div>
      </div>
    </div>

    <!-- SSH 安装弹窗 -->
    <div v-if="showSSHModal" class="modal-overlay" @click.self="closeSSHModal()">
      <div class="modal">
        <div class="modal-header">
          <h3>SSH 远程安装</h3>
          <button @click="closeSSHModal()" class="btn-icon"><X class="w-5 h-5" /></button>
        </div>
        <div class="modal-body space-y-4">
          <div>
            <label class="label">目标主机</label>
            <input v-model="sshForm.host" class="input-dark" placeholder="例：192.168.1.10" />
          </div>
          <div>
            <label class="label">SSH 端口</label>
            <input v-model.number="sshForm.port" type="number" class="input-dark" />
          </div>
          <div>
            <label class="label">用户名</label>
            <input v-model="sshForm.username" class="input-dark" placeholder="root" />
          </div>
          <div>
            <label class="label">认证方式</label>
            <select v-model="sshForm.authType" class="input-dark">
              <option value="password">密码</option>
              <option value="key">密钥</option>
            </select>
          </div>
          <div v-if="sshForm.authType === 'password'">
            <label class="label">密码</label>
            <input v-model="sshForm.password" type="password" class="input-dark" />
          </div>
          <div v-if="sshForm.authType === 'key'">
            <label class="label">私钥（PEM 内容）</label>
            <textarea v-model="sshForm.privateKey" class="input-dark" rows="3" placeholder="-----BEGIN..."></textarea>
          </div>
          <div>
            <label class="label">探针名称</label>
            <input v-model="sshForm.probeName" class="input-dark" placeholder="远程主机上显示的探针名" />
          </div>
          <div>
            <label class="label">探针类型</label>
            <select v-model="sshForm.probeType" class="input-dark">
              <option value="agent">Agent</option>
              <option value="sidecar">Sidecar</option>
            </select>
          </div>
          <div>
            <label class="label">分组</label>
            <input v-model="sshForm.group" class="input-dark" placeholder="可选" />
          </div>
          <div>
            <label class="label">Edge 地址</label>
            <input v-model="sshForm.edgeAddr" class="input-dark" placeholder="edge:50051" />
          </div>
        </div>

        <!-- SSH 安装进度 -->
        <div v-if="installing" class="modal-body border-t border-dark-600 mt-4 pt-4">
          <div class="mb-2 flex items-center justify-between">
            <span class="text-sm text-gray-300">{{ installStatus }}</span>
            <span class="text-xs text-gray-400">{{ installProgress }}%</span>
          </div>
          <div class="w-full bg-dark-600 rounded-full h-2 mb-3">
            <div class="bg-primary-500 h-2 rounded-full transition-all" :style="{ width: installProgress + '%' }"></div>
          </div>
          <div class="max-h-32 overflow-y-auto text-xs font-mono text-gray-400 space-y-0.5">
            <div v-for="(log, i) in installLogs" :key="i">{{ log }}</div>
          </div>
        </div>

        <div class="modal-footer">
          <button @click="closeSSHModal()" :disabled="installing" class="btn btn-secondary">关闭</button>
          <button
            v-if="!installing"
            @click="startSSHInstall()"
            :disabled="!canInstall"
            class="btn btn-primary"
          >
            <Terminal class="w-4 h-4" />
            开始安装
          </button>
        </div>
      </div>
    </div>

    <!-- K8s 部署弹窗 -->
    <div v-if="showK8sModal" class="modal-overlay" @click.self="showK8sModal = false">
      <div class="modal modal-lg">
        <div class="modal-header">
          <h3>Kubernetes 部署</h3>
          <button @click="showK8sModal = false" class="btn-icon"><X class="w-5 h-5" /></button>
        </div>
        <div class="modal-body space-y-4">
          <div>
            <label class="label">部署模式</label>
            <div class="flex gap-3">
              <div
                v-for="mode in k8sDeployModes"
                :key="mode.id"
                @click="k8sForm.deployMode = mode.id"
                class="flex-1 p-3 rounded-xl border cursor-pointer transition-colors"
                :class="k8sForm.deployMode === mode.id
                  ? 'border-primary-500 bg-primary-500/10'
                  : 'border-dark-600 hover:border-dark-500'"
              >
                <p class="font-medium text-white text-sm">{{ mode.name }}</p>
                <p class="text-xs text-gray-400 mt-1">{{ mode.desc }}</p>
              </div>
            </div>
          </div>
          <div>
            <label class="label">命名空间</label>
            <input v-model="k8sForm.namespace" class="input-dark" placeholder="cloudflow" />
          </div>
          <div>
            <label class="label">Edge 地址</label>
            <input v-model="k8sForm.edgeAddr" class="input-dark" placeholder="cloudflow-edge.cloudflow.svc.cluster.local:50051" />
          </div>
          <div>
            <label class="label">镜像仓库</label>
            <input v-model="k8sForm.registry" class="input-dark" placeholder="registry.cloudflow.io" />
          </div>
          <div>
            <label class="label">镜像标签</label>
            <input v-model="k8sForm.tag" class="input-dark" placeholder="latest" />
          </div>
          <div class="flex items-center gap-2">
            <input v-model="k8sForm.enableRBAC" type="checkbox" id="rbac" />
            <label for="rbac" class="text-sm text-gray-300">启用 RBAC</label>
          </div>
          <div class="flex items-center gap-2">
            <input v-model="k8sForm.hostNetwork" type="checkbox" id="hostnet" />
            <label for="hostnet" class="text-sm text-gray-300">Host Network</label>
          </div>
        </div>
        <div class="modal-footer">
          <button @click="showK8sModal = false" class="btn btn-secondary">取消</button>
          <button @click="generateK8sYAML()" class="btn btn-primary">
            <FileCode class="w-4 h-4" />
            生成 YAML
          </button>
          <button @click="deployK8s()" class="btn btn-accent">
            <FolderTree class="w-4 h-4" />
            直接部署
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import {
  Download, ArrowUpCircle, Trash2, X, Terminal, Server,
  CheckCircle, XCircle, FolderTree, Settings, Activity,
  Loader2, Box, Copy, FileCode, HelpCircle, AlertCircle, Database
} from 'lucide-vue-next'

// API base URL（通过 Nginx 代理）
const API_BASE = '/api'

// 响应式状态
const selectedType = ref('all')
const selectedGroup = ref('all')
const showInstallModal = ref(false)
const showGroupModal = ref(false)
const showSSHModal = ref(false)
const showK8sModal = ref(false)
const installing = ref(false)
const installProgress = ref(0)
const installStatus = ref('')
const installLogs = ref([])
const loading = ref(false)

const newProbe = ref({ name: '', type: 'agent', group: '' })
const sshForm = ref({
  host: '', port: 22, username: 'root',
  authType: 'password', password: '', privateKey: '',
  probeName: '', probeType: 'agent', group: '',
  edgeAddr: 'edge:50051'
})
const k8sForm = ref({
  deployMode: 'daemonset', namespace: 'cloudflow',
  apiKey: '', edgeAddr: 'cloudflow-edge.cloudflow.svc.cluster.local:50051',
  registry: 'registry.cloudflow.io', tag: 'latest',
  enableRBAC: true, hostNetwork: true,
  k8sConnectMode: 'in-cluster', kubeconfigPath: '/root/.kube/config',
  k8sApiServer: 'https://kubernetes.default.svc:443',
})

const k8sDeployModes = [
  { id: 'daemonset', name: 'DaemonSet', desc: '在每个 K8s 节点上部署 Pod（推荐）', icon: 'Server' },
  { id: 'node', name: 'Node 安装', desc: '直接在 K8s 节点上安装', icon: 'Terminal' },
  { id: 'ecs', name: 'ECS 独立', desc: '在虚拟机上独立部署', icon: 'Download' }
]

// 探针数据（从后端 API 获取）
const groups = ref([])
const probes = ref([])

const filteredProbes = computed(() => {
  return probes.value.filter(probe => {
    const typeMatch = selectedType.value === 'all' || probe.type === selectedType.value
    const groupMatch = selectedGroup.value === 'all' || probe.group === selectedGroup.value
    return typeMatch && groupMatch
  })
})

const onlineCount = computed(() => probes.value.filter(p => p.status === 'online').length)
const offlineCount = computed(() => probes.value.filter(p => p.status === 'offline' || p.status === 'offline').length)
const warningCount = computed(() => probes.value.filter(p => p.status === 'warning').length)

const canInstall = computed(() => {
  return sshForm.value.host && sshForm.value.username &&
    (sshForm.value.authType === 'password' ? sshForm.value.password : sshForm.value.privateKey)
})

// 获取探针列表（真实 API）
async function fetchProbes() {
  loading.value = true
  try {
    const resp = await fetch(`${API_BASE}/agents`)
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
    const data = await resp.json()
    // 后端返回数组，每个元素包含 name, status, ip, hostname 等
    probes.value = (data || []).map(a => ({
      id: a.name || a.hostname,
      name: a.hostname || a.name,
      type: a.type || 'agent',
      group: a.group || '默认',
      ip: a.ip || '',
      hostname: a.hostname || '',
      status: a.status || 'offline',
      version: a.version || '',
      lastHeartbeat: a.lastHeartbeat || '',
      cpu: a.cpu || 0,
      memory: a.memory || 0,
    }))
    // 提取分组列表
    const gs = [...new Set(probes.value.map(p => p.group))]
    groups.value = gs
  } catch (err) {
    console.warn('[ProbeManage] fetchProbes failed:', err.message)
    probes.value = []
  } finally {
    loading.value = false
  }
}

// 本地安装（调用后端 API 在本地启动 Agent）
async function installProbe() {
  try {
    const resp = await fetch(`${API_BASE}/agents/install`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name: newProbe.value.name,
        type: newProbe.value.type,
        group: newProbe.value.group,
      })
    })
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
    showInstallModal.value = false
    newProbe.value = { name: '', type: 'agent', group: '' }
    await fetchProbes()  // 刷新列表
  } catch (err) {
    alert(`安装失败: ${err.message}`)
  }
}

// SSH 远程安装（调用后端 API，由后端通过 SSH 连接目标服务器执行安装）
async function startSSHInstall() {
  if (!canInstall.value) return
  installing.value = true
  installProgress.value = 0
  installLogs.value = []

  try {
    const resp = await fetch(`${API_BASE}/agents/ssh-install`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        host: sshForm.value.host,
        port: sshForm.value.port,
        username: sshForm.value.username,
        authType: sshForm.value.authType,
        password: sshForm.value.password,
        privateKey: sshForm.value.privateKey,
        probeName: sshForm.value.probeName,
        probeType: sshForm.value.probeType,
        group: sshForm.value.group,
        edgeAddr: sshForm.value.edgeAddr,
      })
    })
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
    const result = await resp.json()
    installProgress.value = 100
    installStatus.value = '安装完成'
    installLogs.value.push(`[SUCCESS] ${result.message || '安装成功'}`)
    await fetchProbes()
  } catch (err) {
    installStatus.value = '安装失败'
    installLogs.value.push(`[ERROR] ${err.message}`)
  } finally {
    installing.value = false
  }
}

// K8s YAML 生成（调用后端 API 生成 YAML）
async function generateK8sYAML() {
  try {
    const resp = await fetch(`${API_BASE}/agents/k8s-yaml`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(k8sForm.value)
    })
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
    const yaml = await resp.text()
    // 下载 YAML 文件
    const blob = new Blob([yaml], { type: 'text/yaml' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'cloudflow-agent.yaml'
    a.click()
    URL.revokeObjectURL(url)
  } catch (err) {
    alert(`生成 YAML 失败: ${err.message}`)
  }
}

// K8s 直接部署（调用后端 API 通过 kubectl 部署）
async function deployK8s() {
  if (!confirm('确认要在集群中部署 Agent 吗？')) return
  try {
    const resp = await fetch(`${API_BASE}/agents/k8s-deploy`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(k8sForm.value)
    })
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
    alert('部署成功！')
    showK8sModal.value = false
    await fetchProbes()
  } catch (err) {
    alert(`部署失败: ${err.message}`)
  }
}

function upgradeProbe(probe) {
  if (!confirm(`确认要升级探针 ${probe.name} 吗？`)) return
  fetch(`${API_BASE}/agents/${probe.id}/upgrade`, { method: 'POST' })
    .then(() => fetchProbes())
    .catch(err => alert(`升级失败: ${err.message}`))
}

function uninstallProbe(probe) {
  if (!confirm(`确认要卸载探针 ${probe.name} 吗？`)) return
  fetch(`${API_BASE}/agents/${probe.id}`, { method: 'DELETE' })
    .then(() => fetchProbes())
    .catch(err => alert(`卸载失败: ${err.message}`))
}

function closeSSHModal() {
  if (installProgress.value === 0 || installProgress.value === 100) {
    showSSHModal.value = false
    installProgress.value = 0
    installStatus.value = ''
    installLogs.value = []
    sshForm.value = {
      host: '', port: 22, username: 'root',
      authType: 'password', password: '', privateKey: '',
      probeName: '', probeType: 'agent', group: '',
      edgeAddr: 'edge:50051'
    }
  }
}

// 页面加载时获取探针列表
onMounted(() => {
  fetchProbes()
})
</script>
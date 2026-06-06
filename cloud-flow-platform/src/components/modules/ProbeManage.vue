<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <h2 class="text-xl font-semibold text-white">探针管理</h2>
      <div class="flex gap-3">
        <button @click="showInstallModal = true" class="px-4 py-2 bg-accent-500 text-dark-300 rounded-lg font-medium hover:bg-accent-400 transition-colors flex items-center gap-2">
          <Download class="w-4 h-4" />
          安装探针
        </button>
        <button @click="showGroupModal = true" class="px-4 py-2 bg-dark-100 text-white rounded-lg font-medium hover:bg-dark-400 transition-colors">
          分组管理
        </button>
      </div>
    </div>

    <!-- 探针列表 -->
    <div class="bg-dark-200 rounded-xl border border-dark-100 overflow-hidden">
      <table class="w-full">
        <thead class="bg-dark-300">
          <tr>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">探针名称</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">类型</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">分组</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">版本</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">状态</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">最后心跳</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">操作</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-dark-100">
          <tr v-for="probe in probes" :key="probe.id" class="hover:bg-dark-100/50">
            <td class="px-4 py-3 text-sm text-white">{{ probe.name }}</td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ probe.type }}</td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ probe.group }}</td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ probe.version }}</td>
            <td class="px-4 py-3">
              <span :class="['px-2 py-0.5 text-xs rounded-full', probe.status === 'online' ? 'bg-green-500/20 text-green-400' : 'bg-red-500/20 text-red-400']">
                {{ probe.status === 'online' ? '在线' : '离线' }}
              </span>
            </td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ probe.lastHeartbeat }}</td>
            <td class="px-4 py-3">
              <div class="flex gap-2">
                <button @click="upgradeProbe(probe)" class="p-1.5 hover:bg-dark-100 rounded text-accent-500" title="升级">
                  <ArrowUpCircle class="w-4 h-4" />
                </button>
                <button @click="uninstallProbe(probe)" class="p-1.5 hover:bg-dark-100 rounded text-red-400" title="卸载">
                  <Trash2 class="w-4 h-4" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 安装弹窗 -->
    <div v-if="showInstallModal" class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div class="bg-dark-200 rounded-xl p-6 w-full max-w-lg border border-dark-100">
        <h3 class="text-lg font-semibold text-white mb-4">安装新探针</h3>
        <div class="space-y-4">
          <div>
            <label class="block text-sm text-gray-400 mb-1">探针名称</label>
            <input v-model="newProbe.name" type="text" class="w-full px-3 py-2 bg-dark-300 border border-dark-100 rounded-lg text-white focus:outline-none focus:border-accent-500" placeholder="输入探针名称" />
          </div>
          <div>
            <label class="block text-sm text-gray-400 mb-1">探针类型</label>
            <select v-model="newProbe.type" class="w-full px-3 py-2 bg-dark-300 border border-dark-100 rounded-lg text-white focus:outline-none focus:border-accent-500">
              <option value="agent">Agent</option>
              <option value="center">Center</option>
              <option value="edge">Edge</option>
            </select>
          </div>
          <div>
            <label class="block text-sm text-gray-400 mb-1">分组</label>
            <input v-model="newProbe.group" type="text" class="w-full px-3 py-2 bg-dark-300 border border-dark-100 rounded-lg text-white focus:outline-none focus:border-accent-500" placeholder="输入分组名称" />
          </div>
          <div>
            <label class="block text-sm text-gray-400 mb-2">安装命令</label>
            <div class="p-3 bg-dark-300 rounded-lg text-sm font-mono text-accent-400 select-all">
              curl -sSL https://install.cloudflow.io/probe.sh | sh -s -- --name {{ newProbe.name || 'YOUR_PROBE_NAME' }} --type {{ newProbe.type || 'agent' }}
            </div>
          </div>
        </div>
        <div class="flex justify-end gap-3 mt-6">
          <button @click="showInstallModal = false" class="px-4 py-2 text-gray-400 hover:text-white">取消</button>
          <button @click="installProbe" class="px-4 py-2 bg-accent-500 text-dark-300 rounded-lg font-medium hover:bg-accent-400">生成命令</button>
        </div>
      </div>
    </div>

    <!-- 分组管理弹窗 -->
    <div v-if="showGroupModal" class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div class="bg-dark-200 rounded-xl p-6 w-full max-w-md border border-dark-100">
        <h3 class="text-lg font-semibold text-white mb-4">分组管理</h3>
        <div class="space-y-2">
          <div v-for="group in probeGroups" :key="group" class="flex items-center justify-between p-3 bg-dark-300 rounded-lg">
            <span class="text-white">{{ group }}</span>
            <span class="text-sm text-gray-400">{{ getGroupCount(group) }} 个探针</span>
          </div>
        </div>
        <div class="flex justify-end mt-6">
          <button @click="showGroupModal = false" class="px-4 py-2 bg-accent-500 text-dark-300 rounded-lg font-medium hover:bg-accent-400">关闭</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import api from '../../api'
import { Download, ArrowUpCircle, Trash2 } from 'lucide-vue-next'

const probes = ref([])
const showInstallModal = ref(false)
const showGroupModal = ref(false)
const newProbe = ref({ name: '', type: 'agent', group: '' })

const probeGroups = ['华北', '华东', '华南', '中心', '边缘']

function getGroupCount(group) {
  return probes.value.filter(p => p.group === group).length
}

function upgradeProbe(probe) {
  alert(`正在升级探针: ${probe.name}`)
}

function uninstallProbe(probe) {
  if (confirm(`确定要卸载探针 ${probe.name} 吗？`)) {
    probes.value = probes.value.filter(p => p.id !== probe.id)
  }
}

function installProbe() {
  showInstallModal.value = false
}
</script>

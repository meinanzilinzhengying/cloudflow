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
            <p class="text-gray-400 text-sm">分组数</p>
            <p class="text-2xl font-bold text-blue-400">{{ groups.length }}</p>
          </div>
          <FolderTree class="w-8 h-8 text-blue-400" />
        </div>
      </div>
    </div>

    <div class="flex items-center justify-between mb-6">
      <div class="flex items-center gap-4">
        <div class="flex items-center gap-2 bg-dark-800 px-4 py-2 rounded-lg border border-dark-600">
          <span class="text-gray-400 text-sm">类型:</span>
          <select 
            v-model="selectedType" 
            class="bg-dark-700 border-none text-white text-sm px-3 py-1 rounded-md focus:outline-none"
          >
            <option value="all">全部类型</option>
            <option value="agent">Agent</option>
            <option value="center">Center</option>
            <option value="edge">Edge</option>
          </select>
        </div>
        <div class="flex items-center gap-2 bg-dark-800 px-4 py-2 rounded-lg border border-dark-600">
          <span class="text-gray-400 text-sm">分组:</span>
          <select 
            v-model="selectedGroup" 
            class="bg-dark-700 border-none text-white text-sm px-3 py-1 rounded-md focus:outline-none"
          >
            <option value="all">全部分组</option>
            <option v-for="group in groups" :key="group" :value="group">{{ group }}</option>
          </select>
        </div>
      </div>
      <div class="flex gap-3">
        <button @click="showGroupModal = true" class="px-4 py-2 bg-dark-700 text-white text-sm font-medium rounded-lg hover:bg-dark-600 transition">
          分组管理
        </button>
        <button @click="showK8sModal = true" class="px-4 py-2 bg-purple-600 text-white text-sm font-medium rounded-lg hover:bg-purple-700 transition flex items-center gap-2">
          <Box class="w-4 h-4" />
          K8s 部署
        </button>
        <button @click="showSSHModal = true" class="px-4 py-2 bg-primary-500 text-white text-sm font-medium rounded-lg hover:bg-primary-600 transition flex items-center gap-2">
          <Terminal class="w-4 h-4" />
          SSH 安装
        </button>
        <button @click="showInstallModal = true" class="px-4 py-2 bg-green-600 text-white text-sm font-medium rounded-lg hover:bg-green-700 transition flex items-center gap-2">
          <Download class="w-4 h-4" />
          本地安装
        </button>
      </div>
    </div>
    
    <!-- 探针列表 -->
    <div class="bg-dark-800 rounded-xl border border-dark-600 overflow-hidden">
      <div class="px-4 py-3 border-b border-dark-600">
        <h3 class="font-semibold text-white">探针列表</h3>
      </div>
      <table class="w-full">
        <thead>
          <tr class="border-b border-dark-600">
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">探针名称</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">类型</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">分组</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">版本</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">IP 地址</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">状态</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">最后心跳</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="probe in filteredProbes" :key="probe.id" class="border-b border-dark-700 hover:bg-dark-700/50 transition">
            <td class="px-4 py-3 text-sm text-white">{{ probe.name }}</td>
            <td class="px-4 py-3 text-sm text-gray-300">{{ probe.type }}</td>
            <td class="px-4 py-3 text-sm text-gray-300">{{ probe.group }}</td>
            <td class="px-4 py-3 text-sm text-gray-300">{{ probe.version }}</td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ probe.ip || '-' }}</td>
            <td class="px-4 py-3">
              <span 
                class="px-2 py-1 text-xs rounded-full"
                :class="probe.status === 'online' ? 'bg-green-500/20 text-green-400' : 'bg-red-500/20 text-red-400'"
              >
                {{ probe.status === 'online' ? '在线' : '离线' }}
              </span>
            </td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ probe.lastHeartbeat }}</td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ probe.flows || '-' }}</td>
            <td class="px-4 py-3">
              <div class="flex gap-2">
                <template v-if="probe.type === 'eBPF'">
                  <button @click="startProbe(probe)" :disabled="probe.status === 'online'" class="p-1.5 hover:bg-dark-600 rounded text-green-400 disabled:opacity-30 disabled:cursor-not-allowed" title="启动">
                    <Play class="w-4 h-4" />
                  </button>
                  <button @click="stopProbe(probe)" :disabled="probe.status !== 'online'" class="p-1.5 hover:bg-dark-600 rounded text-yellow-400 disabled:opacity-30 disabled:cursor-not-allowed" title="停止">
                    <Power class="w-4 h-4" />
                  </button>
                  <button @click="restartProbe(probe)" :disabled="probe.status === 'offline'" class="p-1.5 hover:bg-dark-600 rounded text-blue-400 disabled:opacity-30 disabled:cursor-not-allowed" title="重启">
                    <RotateCcw class="w-4 h-4" />
                  </button>
                </template>
                <template v-else>
                  <button @click="editProbe(probe)" class="p-1.5 hover:bg-dark-600 rounded text-blue-400" title="编辑配置">
                    <Pencil class="w-4 h-4" />
                  </button>
                  <button @click="stopProbe(probe)" :disabled="probe.status !== 'online'" class="p-1.5 hover:bg-dark-600 rounded text-yellow-400 disabled:opacity-30 disabled:cursor-not-allowed" title="停止">
                    <Power class="w-4 h-4" />
                  </button>
                  <button @click="restartProbe(probe)" :disabled="probe.status === 'offline'" class="p-1.5 hover:bg-dark-600 rounded text-green-400 disabled:opacity-30 disabled:cursor-not-allowed" title="重启">
                    <RotateCcw class="w-4 h-4" />
                  </button>
                  <button @click="upgradeProbe(probe)" class="p-1.5 hover:bg-dark-600 rounded text-primary-400" title="升级">
                    <ArrowUpCircle class="w-4 h-4" />
                  </button>
                  <button @click="uninstallProbe(probe)" class="p-1.5 hover:bg-dark-600 rounded text-red-400" title="卸载">
                    <Trash2 class="w-4 h-4" />
                  </button>
                </template>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    
    <!-- K8s 部署弹窗 -->
    <div v-if="showK8sModal" class="fixed inset-0 bg-dark-900/80 flex items-center justify-center z-50">
      <div class="bg-dark-800 rounded-xl p-6 w-full max-w-3xl border border-dark-600 max-h-[90vh] overflow-y-auto">
        <div class="flex items-center justify-between mb-4">
          <div class="flex items-center gap-3">
            <div class="w-10 h-10 bg-purple-500/20 rounded-lg flex items-center justify-center">
              <Kubernetes class="w-5 h-5 text-purple-400" />
            </div>
            <div>
              <h3 class="text-lg font-semibold text-white">Kubernetes 部署探针</h3>
              <p class="text-sm text-gray-400">通过 DaemonSet 在集群每个节点上部署探针</p>
            </div>
          </div>
          <button @click="showK8sModal = false" class="text-gray-400 hover:text-white">
            <X class="w-5 h-5" />
          </button>
        </div>
        
        <!-- 部署方式选择 -->
        <div class="mb-6">
          <h4 class="text-sm font-medium text-white mb-3">选择部署方式</h4>
          <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
            <div 
              v-for="mode in k8sDeployModes"
              :key="mode.id"
              @click="k8sForm.deployMode = mode.id"
              class="cursor-pointer p-4 rounded-lg border-2 transition-all"
              :class="k8sForm.deployMode === mode.id ? 'border-purple-500 bg-purple-500/10' : 'border-dark-600 hover:border-dark-500'"
            >
              <div class="flex items-center gap-2 mb-2">
                <component :is="mode.icon" class="w-5 h-5" :class="mode.id === 'daemonset' ? 'text-purple-400' : mode.id === 'node' ? 'text-blue-400' : 'text-green-400'" />
                <span class="font-medium text-white">{{ mode.name }}</span>
              </div>
              <p class="text-xs text-gray-400">{{ mode.desc }}</p>
            </div>
          </div>
        </div>
        
        <!-- K8s 配置 -->
        <div class="space-y-4 mb-6">
          <div class="bg-dark-700/50 rounded-lg p-4">
            <h4 class="text-sm font-medium text-white mb-3 flex items-center gap-2">
              <Settings class="w-4 h-4" />
              基本配置
            </h4>
            <div class="grid grid-cols-2 gap-4">
              <div>
                <label class="block text-sm text-gray-400 mb-1">命名空间</label>
                <input 
                  v-model="k8sForm.namespace" 
                  type="text" 
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-purple-500"
                  placeholder="cloudflow"
                />
              </div>
              <div>
                <label class="block text-sm text-gray-400 mb-1">API Key</label>
                <input 
                  v-model="k8sForm.apiKey" 
                  type="password" 
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-purple-500"
                  placeholder="输入CloudFlow API Key"
                />
              </div>
              <div class="col-span-2">
                <label class="block text-sm text-gray-400 mb-1">Edge 服务地址</label>
                <input 
                  v-model="k8sForm.edgeAddr" 
                  type="text" 
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-purple-500"
                  placeholder="cloudflow-edge.cloudflow.svc.cluster.local:50051"
                />
              </div>
            </div>
          </div>
          
          <!-- K8s 集群连接配置 -->
          <div class="bg-dark-700/50 rounded-lg p-4">
            <h4 class="text-sm font-medium text-white mb-3 flex items-center gap-2">
              <Box class="w-4 h-4" />
              K8s 集群连接配置
            </h4>
            <p class="text-xs text-gray-400 mb-4">
              配置探针如何连接 K8s API 来获取容器信息（Pod、Service、Namespace 等）
            </p>
            <div class="grid grid-cols-2 gap-4">
              <div class="col-span-2">
                <label class="block text-sm text-gray-400 mb-1">连接模式</label>
                <select 
                  v-model="k8sForm.k8sConnectMode" 
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-purple-500"
                >
                  <option value="in-cluster">集群内 (In-Cluster) - 推荐（使用ServiceAccount）</option>
                  <option value="kubeconfig">Kubeconfig - 指定 kubeconfig 文件路径</option>
                  <option value="manual">手动配置 - 输入 API 地址和 Token</option>
                </select>
              </div>
              
              <div v-if="k8sForm.k8sConnectMode === 'kubeconfig'" class="col-span-2">
                <label class="block text-sm text-gray-400 mb-1">Kubeconfig 文件路径</label>
                <input 
                  v-model="k8sForm.kubeconfigPath" 
                  type="text" 
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-purple-500"
                  placeholder="/root/.kube/config"
                />
              </div>
              
              <template v-if="k8sForm.k8sConnectMode === 'manual'">
                <div>
                  <label class="block text-sm text-gray-400 mb-1">K8s API 地址</label>
                  <input 
                    v-model="k8sForm.k8sApiServer" 
                    type="text" 
                    class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-purple-500"
                    placeholder="https://kubernetes.default.svc:443"
                  />
                </div>
                <div>
                  <label class="block text-sm text-gray-400 mb-1">Token</label>
                  <input 
                    v-model="k8sForm.k8sToken" 
                    type="password" 
                    class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-purple-500"
                    placeholder="输入 Token"
                  />
                </div>
                <div class="col-span-2">
                  <label class="block text-sm text-gray-400 mb-1">CA 证书 (可选)</label>
                  <textarea 
                    v-model="k8sForm.k8sCaCert" 
                    rows="3"
                    class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-purple-500 font-mono text-xs"
                    placeholder="-----BEGIN CERTIFICATE-----&#10;...&#10;-----END CERTIFICATE-----"
                  ></textarea>
                </div>
              
    

</template>
            </div>
          </div>
          
          <!-- K8s 数据采集配置 -->
          <div class="bg-dark-700/50 rounded-lg p-4">
            <h4 class="text-sm font-medium text-white mb-3 flex items-center gap-2">
              <Database class="w-4 h-4" />
              K8s 数据采集配置
            </h4>
            <div class="grid grid-cols-2 gap-4">
              <div class="col-span-2">
                <label class="block text-sm text-gray-400 mb-1">包含命名空间</label>
                <input 
                  v-model="k8sForm.includeNamespaces" 
                  type="text" 
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-purple-500"
                  placeholder="留空表示监控所有；多个用逗号分隔：default,prod,staging"
                />
              </div>
              <div class="col-span-2">
                <label class="block text-sm text-gray-400 mb-1">排除命名空间</label>
                <input 
                  v-model="k8sForm.excludeNamespaces" 
                  type="text" 
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-purple-500"
                  placeholder="kube-system,kube-public,kube-node-lease"
                />
              </div>
              <div>
                <label class="block text-sm text-gray-400 mb-1">同步间隔 (秒)</label>
                <input 
                  v-model.number="k8sForm.syncInterval" 
                  type="number" 
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-purple-500"
                  placeholder="30"
                />
              </div>
              <div class="col-span-2 space-y-2">
                <h5 class="text-xs font-medium text-gray-300">采集资源</h5>
                <div class="grid grid-cols-3 gap-2">
                  <label class="flex items-center gap-2 text-xs text-gray-300">
                    <input type="checkbox" v-model="k8sForm.collectPods" class="rounded bg-dark-700 border-dark-600" />
                    Pods
                  </label>
                  <label class="flex items-center gap-2 text-xs text-gray-300">
                    <input type="checkbox" v-model="k8sForm.collectServices" class="rounded bg-dark-700 border-dark-600" />
                    Services
                  </label>
                  <label class="flex items-center gap-2 text-xs text-gray-300">
                    <input type="checkbox" v-model="k8sForm.collectDeployments" class="rounded bg-dark-700 border-dark-600" />
                    Deployments
                  </label>
                  <label class="flex items-center gap-2 text-xs text-gray-300">
                    <input type="checkbox" v-model="k8sForm.collectReplicasets" class="rounded bg-dark-700 border-dark-600" />
                    ReplicaSets
                  </label>
                  <label class="flex items-center gap-2 text-xs text-gray-300">
                    <input type="checkbox" v-model="k8sForm.collectStatefulsets" class="rounded bg-dark-700 border-dark-600" />
                    StatefulSets
                  </label>
                  <label class="flex items-center gap-2 text-xs text-gray-300">
                    <input type="checkbox" v-model="k8sForm.collectDaemonsets" class="rounded bg-dark-700 border-dark-600" />
                    DaemonSets
                  </label>
                  <label class="flex items-center gap-2 text-xs text-gray-300">
                    <input type="checkbox" v-model="k8sForm.collectJobs" class="rounded bg-dark-700 border-dark-600" />
                    Jobs
                  </label>
                  <label class="flex items-center gap-2 text-xs text-gray-300">
                    <input type="checkbox" v-model="k8sForm.collectCronjobs" class="rounded bg-dark-700 border-dark-600" />
                    CronJobs
                  </label>
                  <label class="flex items-center gap-2 text-xs text-gray-300">
                    <input type="checkbox" v-model="k8sForm.collectNamespaces" class="rounded bg-dark-700 border-dark-600" />
                    Namespaces
                  </label>
                </div>
              </div>
            </div>
          </div>
          
          <!-- DaemonSet 配置 -->
          <div v-if="k8sForm.deployMode === 'daemonset'" class="bg-dark-700/50 rounded-lg p-4">
            <h4 class="text-sm font-medium text-white mb-3 flex items-center gap-2">
              <Server class="w-4 h-4" />
              DaemonSet 配置
            </h4>
            <div class="grid grid-cols-2 gap-4">
              <div>
                <label class="block text-sm text-gray-400 mb-1">镜像仓库</label>
                <input 
                  v-model="k8sForm.registry" 
                  type="text" 
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-purple-500"
                  placeholder="registry.cloudflow.io"
                />
              </div>
              <div>
                <label class="block text-sm text-gray-400 mb-1">镜像标签</label>
                <input 
                  v-model="k8sForm.tag" 
                  type="text" 
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-purple-500"
                  placeholder="latest"
                />
              </div>
              <div class="col-span-2">
                <div class="flex items-center gap-4 mt-2">
                  <label class="flex items-center gap-2 text-sm text-gray-300">
                    <input type="checkbox" v-model="k8sForm.enableRBAC" class="rounded bg-dark-700 border-dark-600" />
                    启用 RBAC
                  </label>
                  <label class="flex items-center gap-2 text-sm text-gray-300">
                    <input type="checkbox" v-model="k8sForm.hostNetwork" class="rounded bg-dark-700 border-dark-600" />
                    主机网络模式
                  </label>
                </div>
              </div>
            </div>
          </div>
        </div>
        
        <!-- 生成命令 -->
        <div class="mb-6">
          <div class="flex items-center justify-between mb-2">
            <h4 class="text-sm font-medium text-white">部署命令</h4>
            <button @click="copyK8sCommand" class="text-xs text-purple-400 hover:text-purple-300 flex items-center gap-1">
              <Copy class="w-3 h-3" />
              复制
            </button>
          </div>
          <div class="p-3 bg-dark-900 rounded-lg font-mono text-xs text-purple-300 overflow-x-auto">
            {{ k8sDeployCommand }}
          </div>
        </div>
        
        <!-- 权限说明 -->
        <div class="bg-amber-500/10 border border-amber-500/30 rounded-lg p-4 mb-6">
          <h5 class="text-sm font-medium text-amber-400 mb-2 flex items-center gap-2">
            <AlertCircle class="w-4 h-4" />
            权限说明
          </h5>
          <ul class="text-xs text-gray-300 space-y-1">
            <li>• 需要 Pod 的 get/list/watch 权限</li>
            <li>• 推荐读取 Service/Node/Namespace 信息</li>
            <li>• 不需要读取 Secret/ConfigMap 内容</li>
            <li>• 使用 ServiceAccount + ClusterRole 最小权限原则</li>
          </ul>
        </div>
        
        <div class="flex justify-end gap-3">
          <button @click="showK8sModal = false" class="px-4 py-2 text-gray-400 hover:text-white transition">
            取消
          </button>
          <button 
            @click="generateK8sYAML"
            class="px-6 py-2 bg-purple-600 text-white rounded-lg font-medium hover:bg-purple-700 transition flex items-center gap-2"
          >
            <FileCode class="w-4 h-4" />
            生成 YAML
          </button>
          <button 
            @click="openK8sGuide"
            class="px-6 py-2 bg-dark-700 text-white rounded-lg font-medium hover:bg-dark-600 transition flex items-center gap-2"
          >
            <HelpCircle class="w-4 h-4" />
            查看文档
          </button>
        </div>
      </div>
    </div>
    
    <!-- SSH 安装弹窗 -->
    <div v-if="showSSHModal" class="fixed inset-0 bg-dark-900/80 flex items-center justify-center z-50">
      <div class="bg-dark-800 rounded-xl p-6 w-full max-w-2xl border border-dark-600 max-h-[90vh] overflow-y-auto">
        <div class="flex items-center justify-between mb-4">
          <div class="flex items-center gap-3">
            <div class="w-10 h-10 bg-primary-500/20 rounded-lg flex items-center justify-center">
              <Terminal class="w-5 h-5 text-primary-400" />
            </div>
            <div>
              <h3 class="text-lg font-semibold text-white">SSH 远程安装探针</h3>
              <p class="text-sm text-gray-400">通过 SSH 连接远程服务器并自动安装探针</p>
            </div>
          </div>
          <button @click="closeSSHModal" class="text-gray-400 hover:text-white">
            <X class="w-5 h-5" />
          </button>
        </div>
        
        <!-- SSH 连接信息 -->
        <div class="space-y-4">
          <div class="bg-dark-700/50 rounded-lg p-4 mb-4">
            <h4 class="text-sm font-medium text-white mb-3 flex items-center gap-2">
              <Server class="w-4 h-4" />
              连接信息
            </h4>
            <div class="grid grid-cols-2 gap-4">
              <div>
                <label class="block text-sm text-gray-400 mb-1">主机 IP *</label>
                <input 
                  v-model="sshForm.host" 
                  type="text" 
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500"
                  placeholder="192.168.1.100"
                />
              </div>
              <div>
                <label class="block text-sm text-gray-400 mb-1">端口 *</label>
                <input 
                  v-model="sshForm.port" 
                  type="number" 
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500"
                  placeholder="22"
                />
              </div>
              <div>
                <label class="block text-sm text-gray-400 mb-1">用户名 *</label>
                <input 
                  v-model="sshForm.username" 
                  type="text" 
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500"
                  placeholder="root"
                />
              </div>
              <div>
                <label class="block text-sm text-gray-400 mb-1">认证方式</label>
                <select 
                  v-model="sshForm.authType" 
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500"
                >
                  <option value="password">密码认证</option>
                  <option value="key">SSH 密钥</option>
                </select>
              </div>
              <div :class="sshForm.authType === 'key' ? 'col-span-2' : ''">
                <label class="block text-sm text-gray-400 mb-1">
                  {{ sshForm.authType === 'password' ? '密码 *' : '私钥内容 *' }}
                </label>
                <textarea 
                  v-if="sshForm.authType === 'key'"
                  v-model="sshForm.privateKey"
                  rows="5"
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500 font-mono text-sm"
                  placeholder="-----BEGIN RSA PRIVATE KEY-----"
                ></textarea>
                <input 
                  v-else
                  v-model="sshForm.password" 
                  type="password" 
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500"
                  placeholder="输入密码"
                />
              </div>
            </div>
          </div>
          
          <div class="bg-dark-700/50 rounded-lg p-4 mb-4">
            <h4 class="text-sm font-medium text-white mb-3 flex items-center gap-2">
              <Settings class="w-4 h-4" />
              探针配置
            </h4>
            <div class="grid grid-cols-2 gap-4">
              <div>
                <label class="block text-sm text-gray-400 mb-1">探针名称 *</label>
                <input 
                  v-model="sshForm.probeName" 
                  type="text" 
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500"
                  placeholder="agent-prod-01"
                />
              </div>
              <div>
                <label class="block text-sm text-gray-400 mb-1">探针类型</label>
                <select 
                  v-model="sshForm.probeType" 
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500"
                >
                  <option value="agent">Agent</option>
                  <option value="edge">Edge</option>
                </select>
              </div>
              <div>
                <label class="block text-sm text-gray-400 mb-1">分组</label>
                <input 
                  v-model="sshForm.group" 
                  type="text" 
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500"
                  placeholder="华北/华东/华南"
                />
              </div>
              <div>
                <label class="block text-sm text-gray-400 mb-1">中心地址</label>
                <input 
                  v-model="sshForm.edgeAddr" 
                  type="text" 
                  class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500"
                  placeholder="edge:50051"
                />
              </div>
            </div>
          </div>
          
          <!-- 安装进度 -->
          <div v-if="installProgress > 0" class="bg-dark-700/50 rounded-lg p-4 mb-4">
            <h4 class="text-sm font-medium text-white mb-3 flex items-center gap-2">
              <Activity class="w-4 h-4" />
              安装进度
            </h4>
            <div class="mb-2 flex justify-between text-sm">
              <span class="text-gray-400">{{ installStatus }}</span>
              <span class="text-primary-400">{{ installProgress }}%</span>
            </div>
            <div class="h-2 bg-dark-700 rounded-full overflow-hidden">
              <div 
                class="h-full bg-primary-500 transition-all duration-300"
                :style="{ width: installProgress + '%' }"
              ></div>
            </div>
            <div class="mt-3 text-xs text-gray-500 font-mono max-h-24 overflow-y-auto">
              <div v-for="(log, idx) in installLogs" :key="idx" class="py-0.5">
                {{ log }}
              </div>
            </div>
          </div>
        </div>
        
        <div class="flex justify-end gap-3 mt-6">
          <button @click="closeSSHModal" class="px-4 py-2 text-gray-400 hover:text-white transition">
            {{ installProgress > 0 ? '关闭' : '取消' }}
          </button>
          <button 
            @click="startSSHInstall" 
            :disabled="installing || !canInstall"
            class="px-6 py-2 bg-primary-500 text-white rounded-lg font-medium hover:bg-primary-600 transition flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <Loader2 v-if="installing" class="w-4 h-4 animate-spin" />
            {{ installing ? '安装中...' : '开始安装' }}
          </button>
        </div>
      </div>
    </div>
    
    <!-- 本地安装弹窗 -->
    <div v-if="showInstallModal" class="fixed inset-0 bg-dark-900/80 flex items-center justify-center z-50">
      <div class="bg-dark-800 rounded-xl p-6 w-full max-w-lg border border-dark-600">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-semibold text-white">本地安装探针</h3>
          <button @click="showInstallModal = false" class="text-gray-400 hover:text-white">
            <X class="w-5 h-5" />
          </button>
        </div>
        <div class="space-y-4">
          <div>
            <label class="block text-sm text-gray-400 mb-1">探针名称</label>
            <input v-model="newProbe.name" type="text" class="w-full px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500" placeholder="输入探针名称" />
          </div>
          <div>
            <label class="block text-sm text-gray-400 mb-1">探针类型</label>
            <select v-model="newProbe.type" class="w-full px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500">
              <option value="agent">Agent</option>
              <option value="center">Center</option>
              <option value="edge">Edge</option>
            </select>
          </div>
          <div>
            <label class="block text-sm text-gray-400 mb-1">分组</label>
            <input v-model="newProbe.group" type="text" class="w-full px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500" placeholder="输入分组名称" />
          </div>
          <div>
            <label class="block text-sm text-gray-400 mb-2">安装命令</label>
            <div class="p-3 bg-dark-700 rounded-lg text-sm font-mono text-primary-400 select-all">
              curl -sSL https://install.cloudflow.io/probe.sh | sh -s -- --name {{ newProbe.name || 'YOUR_PROBE_NAME' }} --type {{ newProbe.type || 'agent' }}
            </div>
          </div>
        </div>
        <div class="flex justify-end gap-3 mt-6">
          <button @click="showInstallModal = false" class="px-4 py-2 text-gray-400 hover:text-white transition">取消</button>
          <button @click="installProbe" class="px-4 py-2 bg-primary-500 text-white rounded-lg font-medium hover:bg-primary-600 transition">完成</button>
        </div>
      </div>
    </div>
    
    <!-- 分组管理弹窗 -->
    <div v-if="showGroupModal" class="fixed inset-0 bg-dark-900/80 flex items-center justify-center z-50">
      <div class="bg-dark-800 rounded-xl p-6 w-full max-w-md border border-dark-600">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-semibold text-white">分组管理</h3>
          <button @click="showGroupModal = false" class="text-gray-400 hover:text-white">
            <X class="w-5 h-5" />
          </button>
        </div>
        <!-- 现有分组列表 -->
        <div v-if="groups.length > 0" class="space-y-2 mb-4 max-h-48 overflow-y-auto">
          <div v-for="group in groups" :key="group" class="flex items-center justify-between p-3 bg-dark-700 rounded-lg group-item">
            <div class="flex items-center gap-2 flex-1 min-w-0">
              <span v-if="editingGroup !== group" class="text-white truncate">{{ group }}</span>
              <input v-else v-model="editingGroupName" @keyup.enter="saveGroupRename(group)" @keyup.escape="cancelGroupRename"
                type="text"
                class="flex-1 px-2 py-1 bg-dark-600 border border-primary-500 rounded text-white text-sm focus:outline-none" />
              <span class="text-sm text-gray-400 shrink-0">{{ getGroupCount(group) }} 个探针</span>
            </div>
            <div class="flex gap-1 ml-2 shrink-0">
              <button v-if="editingGroup !== group" @click="startRenameGroup(group)" class="p-1 hover:bg-dark-500 rounded text-blue-400" title="重命名">
                <Pencil class="w-3.5 h-3.5" />
              </button>
              <button v-if="editingGroup === group" @click="saveGroupRename(group)" class="p-1 hover:bg-dark-500 rounded text-green-400" title="保存">
                <CheckCircle class="w-3.5 h-3.5" />
              </button>
              <button v-if="editingGroup !== group && group !== '默认'" @click="deleteGroup(group)" class="p-1 hover:bg-dark-500 rounded text-red-400" title="删除">
                <Trash2 class="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
        </div>
        <div v-else class="text-center py-6 text-gray-500 text-sm mb-4">暂无分组，请创建新分组</div>
        <!-- 新建分组 -->
        <div class="mt-4 flex gap-2">
          <input v-model="newGroup" type="text" class="flex-1 px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500" placeholder="新建分组名称" @keyup.enter="addGroup" />
          <button @click="addGroup" class="px-4 py-2 bg-primary-500 text-white rounded-lg hover:bg-primary-600 transition">添加</button>
        </div>
        <div class="flex justify-end mt-6">
          <button @click="showGroupModal = false" class="px-4 py-2 bg-primary-500 text-white rounded-lg font-medium hover:bg-primary-600 transition">关闭</button>
        </div>
      </div>
    </div>

    <!-- 编辑探针弹窗 -->
    <div v-if="showEditModal" class="fixed inset-0 bg-dark-900/80 flex items-center justify-center z-50">
      <div class="bg-dark-800 rounded-xl p-6 w-full max-w-2xl border border-dark-600 max-h-[90vh] overflow-y-auto">
        <div class="flex items-center justify-between mb-4">
          <div class="flex items-center gap-3">
            <div class="w-10 h-10 bg-blue-500/20 rounded-lg flex items-center justify-center">
              <Pencil class="w-5 h-5 text-blue-400" />
            </div>
            <div>
              <h3 class="text-lg font-semibold text-white">编辑探针配置</h3>
              <p class="text-sm text-gray-400">{{ editTarget?.name }} ({{ editTarget?.ip }})</p>
            </div>
          </div>
          <button @click="showEditModal = false" class="text-gray-400 hover:text-white">
            <X class="w-5 h-5" />
          </button>
        </div>

        <div v-if="editTarget" class="space-y-4">
          <!-- 基本信息 -->
          <div class="bg-dark-700/50 rounded-lg p-4">
            <h4 class="text-sm font-medium text-white mb-3 flex items-center gap-2">
              <Settings class="w-4 h-4" />
              基本信息
            </h4>
            <div class="grid grid-cols-2 gap-4">
              <div>
                <label class="block text-sm text-gray-400 mb-1">探针名称</label>
                <input v-model="editForm.name" type="text" class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-blue-500" />
              </div>
              <div>
                <label class="block text-sm text-gray-400 mb-1">所属分组</label>
                <select v-model="editForm.group" class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-blue-500">
                  <option v-for="g in groups" :key="g" :value="g">{{ g }}</option>
                  <option value="">新建...</option>
                </select>
              </div>
              <div v-if="!groups.includes(editForm.group)">
                <label class="block text-sm text-gray-400 mb-1">新分组名</label>
                <input v-model="editForm.newGroup" type="text" class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-blue-500" placeholder="输入新分组名称" />
              </div>
            </div>
          </div>

          <!-- 资源限制 -->
          <div class="bg-dark-700/50 rounded-lg p-4">
            <h4 class="text-sm font-medium text-white mb-3 flex items-center gap-2">
              <Server class="w-4 h-4" />
              资源限制
            </h4>
            <div class="grid grid-cols-2 gap-4">
              <div>
                <label class="block text-sm text-gray-400 mb-1">最大 CPU 核数</label>
                <input v-model.number="editForm.maxCpuCore" type="number" min="0" step="1" class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-blue-500" placeholder="2" />
              </div>
              <div>
                <label class="block text-sm text-gray-400 mb-1">最大内存 (MB)</label>
                <input v-model.number="editForm.maxMemoryMb" type="number" min="0" step="128" class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-blue-500" placeholder="2048" />
              </div>
            </div>
          </div>

          <!-- 采集协议配置 -->
          <div class="bg-dark-700/50 rounded-lg p-4">
            <h4 class="text-sm font-medium text-white mb-3 flex items-center gap-2">
              <Activity class="w-4 h-4" />
              采集协议 / 指标开关
            </h4>
            <div class="space-y-3">
              <p class="text-xs text-gray-400">选择需要启用的数据采集模块：</p>
              <div class="grid grid-cols-3 gap-3">
                <label class="flex items-center gap-2 p-2 rounded-lg border border-dark-600 cursor-pointer hover:bg-dark-600 transition" :class="editForm.collect.cpu ? 'border-green-500/50 bg-green-500/5' : ''">
                  <input type="checkbox" v-model="editForm.collect.cpu" class="rounded bg-dark-700 border-dark-600" />
                  <span class="text-sm text-gray-200">CPU 指标</span>
                </label>
                <label class="flex items-center gap-2 p-2 rounded-lg border border-dark-600 cursor-pointer hover:bg-dark-600 transition" :class="editForm.collect.memory ? 'border-green-500/50 bg-green-500/5' : ''">
                  <input type="checkbox" v-model="editForm.collect.memory" class="rounded bg-dark-700 border-dark-600" />
                  <span class="text-sm text-gray-200">内存指标</span>
                </label>
                <label class="flex items-center gap-2 p-2 rounded-lg border border-dark-600 cursor-pointer hover:bg-dark-600 transition" :class="editForm.collect.network ? 'border-green-500/50 bg-green-500/5' : ''">
                  <input type="checkbox" v-model="editForm.collect.network" class="rounded bg-dark-700 border-dark-600" />
                  <span class="text-sm text-gray-200">网络流量</span>
                </label>
                <label class="flex items-center gap-2 p-2 rounded-lg border border-dark-600 cursor-pointer hover:bg-dark-600 transition" :class="editForm.collect.disk ? 'border-green-500/50 bg-green-500/5' : ''">
                  <input type="checkbox" v-model="editForm.collect.disk" class="rounded bg-dark-700 border-dark-600" />
                  <span class="text-sm text-gray-200">磁盘 IO</span>
                </label>
                <label class="flex items-center gap-2 p-2 rounded-lg border border-dark-600 cursor-pointer hover:bg-dark-600 transition" :class="editForm.ebpf.enabled ? 'border-purple-500/50 bg-purple-500/5' : ''">
                  <input type="checkbox" v-model="editForm.ebpf.enabled" class="rounded bg-dark-700 border-dark-600" />
                  <span class="text-sm text-gray-200">eBPF 采集</span>
                </label>
                <label class="flex items-center gap-2 p-2 rounded-lg border border-dark-600 cursor-pointer hover:bg-dark-600 transition" :class="editForm.ebpf.tcpMetrics.enabled ? 'border-purple-500/50 bg-purple-500/5' : ''">
                  <input type="checkbox" v-model="editForm.ebpf.tcpMetrics.enabled" :disabled="!editForm.ebpf.enabled" class="rounded bg-dark-700 border-dark-600" />
                  <span class="text-sm text-gray-200">TCP 指标</span>
                </label>
                <label class="flex items-center gap-2 p-2 rounded-lg border border-dark-600 cursor-pointer hover:bg-dark-600 transition" :class="editForm.ebpf.httpMetrics.enabled ? 'border-purple-500/50 bg-purple-500/5' : ''">
                  <input type="checkbox" v-model="editForm.ebpf.httpMetrics.enabled" :disabled="!editForm.ebpf.enabled" class="rounded bg-dark-700 border-dark-600" />
                  <span class="text-sm text-gray-200">HTTP 指标</span>
                </label>
                <label class="flex items-center gap-2 p-2 rounded-lg border border-dark-600 cursor-pointer hover:bg-dark-600 transition" :class="editForm.ebpf.protocolParsing.enabled ? 'border-purple-500/50 bg-purple-500/5' : ''">
                  <input type="checkbox" v-model="editForm.ebpf.protocolParsing.enabled" :disabled="!editForm.ebpf.enabled" class="rounded bg-dark-700 border-dark-600" />
                  <span class="text-sm text-gray-200">协议解析</span>
                </label>
              </div>
            </div>
          </div>

          <!-- eBPF 资源限制 -->
          <div v-if="editForm.ebpf.enabled" class="bg-dark-700/50 rounded-lg p-4">
            <h4 class="text-sm font-medium text-white mb-3 flex items-center gap-2">
              <Database class="w-4 h-4" />
              eBPF 资源限制
            </h4>
            <div class="grid grid-cols-2 gap-4">
              <div>
                <label class="block text-sm text-gray-400 mb-1">eBPF 最大 CPU 核数</label>
                <input v-model.number="editForm.ebpf.resourceLimit.maxCpuCore" type="number" min="0" step="1" class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-blue-500" placeholder="0=不限制" />
              </div>
              <div>
                <label class="block text-sm text-gray-400 mb-1">eBPF 最大内存 (MB)</label>
                <input v-model.number="editForm.ebpf.resourceLimit.maxMemoryMb" type="number" min="0" step="128" class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-blue-500" placeholder="0=不限制" />
              </div>
            </div>
          </div>
        </div>

        <div class="flex justify-end gap-3 mt-6">
          <button @click="showEditModal = false" class="px-4 py-2 text-gray-400 hover:text-white transition">取消</button>
          <button @click="saveProbeEdit" :disabled="savingEdit" class="px-6 py-2 bg-blue-600 text-white rounded-lg font-medium hover:bg-blue-700 transition disabled:opacity-50 flex items-center gap-2">
            <Loader2 v-if="savingEdit" class="w-4 h-4 animate-spin" />
            {{ savingEdit ? '保存中...' : '保存配置' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>

// 简单通知函数（替代ElMessage）
function notify(type, message) {
  if (type === 'success') { alert('✅ ' + message); }
  else if (type === 'error') { alert('❌ ' + message); }
  else { alert(message); }
}

import { ref, computed, onMounted, onUnmounted } from 'vue'
import {
  Download,
  ArrowUpCircle,
  Trash2,
  X,
  Terminal,
  Server,
  CheckCircle,
  XCircle,
  FolderTree,
  Settings,
  Activity,
  Loader2,
  Box,
  Copy,
  FileCode,
  HelpCircle,
  AlertCircle,
  Database,
  Pencil,
  Power,
  RotateCcw
} from 'lucide-vue-next'
import api from '../../api'

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

const k8sDeployModes = [
  { 
    id: 'daemonset', 
    name: 'DaemonSet', 
    desc: '在每个K8s节点上部署Pod（推荐）',
    icon: 'Server'
  },
  { 
    id: 'node', 
    name: 'Node安装', 
    desc: '直接在K8s节点上安装',
    icon: 'Terminal'
  },
  { 
    id: 'ecs', 
    name: 'ECS独立', 
    desc: '在虚拟机上独立部署',
    icon: 'Download'
  }
]

const k8sForm = ref({
  deployMode: 'daemonset',
  namespace: 'cloudflow',
  apiKey: '',
  edgeAddr: 'cloudflow-edge.cloudflow.svc.cluster.local:50051',
  registry: 'registry.cloudflow.io',
  tag: 'latest',
  enableRBAC: true,
  hostNetwork: true,
  // K8s连接配置
  k8sConnectMode: 'in-cluster',
  kubeconfigPath: '/root/.kube/config',
  k8sApiServer: 'https://kubernetes.default.svc:443',
  k8sToken: '',
  k8sCaCert: '',
  // K8s数据采集配置
  includeNamespaces: '',
  excludeNamespaces: 'kube-system,kube-public,kube-node-lease',
  syncInterval: 30,
  collectPods: true,
  collectServices: true,
  collectDeployments: true,
  collectReplicasets: true,
  collectStatefulsets: true,
  collectDaemonsets: true,
  collectJobs: true,
  collectCronjobs: true,
  collectNamespaces: true
})

const k8sDeployCommand = computed(() => {
  let cmd = `# 一键部署 CloudFlow Agent
curl -sSL https://raw.githubusercontent.com/meinanzilinzhengying/cloudflow/main/cloud-flow-agent/deployments/k8s/deploy.sh | bash -s -- \\
  --namespace ${k8sForm.value.namespace} \\
  --api-key ${k8sForm.value.apiKey || 'YOUR_API_KEY'} \\
  --edge-addr ${k8sForm.value.edgeAddr} \\
  --registry ${k8sForm.value.registry} \\
  --tag ${k8sForm.value.tag} \\
  --k8s-connect-mode ${k8sForm.value.k8sConnectMode} \\
  --sync-interval ${k8sForm.value.syncInterval}`
  
  if (k8sForm.value.includeNamespaces) {
    cmd += ` \\
  --include-namespaces "${k8sForm.value.includeNamespaces}"`
  }
  if (k8sForm.value.excludeNamespaces) {
    cmd += ` \\
  --exclude-namespaces "${k8sForm.value.excludeNamespaces}"`
  }
  
  const resources = []
  if (k8sForm.value.collectPods) resources.push('pods')
  if (k8sForm.value.collectServices) resources.push('services')
  if (k8sForm.value.collectDeployments) resources.push('deployments')
  if (k8sForm.value.collectReplicasets) resources.push('replicasets')
  if (k8sForm.value.collectStatefulsets) resources.push('statefulsets')
  if (k8sForm.value.collectDaemonsets) resources.push('daemonsets')
  if (k8sForm.value.collectJobs) resources.push('jobs')
  if (k8sForm.value.collectCronjobs) resources.push('cronjobs')
  if (k8sForm.value.collectNamespaces) resources.push('namespaces')
  
  if (resources.length > 0) {
    cmd += ` \\
  --collect-resources "${resources.join(',')}"`
  }
  
  return cmd
})

function copyK8sCommand() {
  navigator.clipboard.writeText(k8sDeployCommand.value).then(() => {
    alert('命令已复制到剪贴板')
  }).catch(() => {
    alert('复制失败，请手动复制')
  })
}

function generateK8sYAML() {
  alert('YAML生成功能需要后端支持，将在后续版本中实现')
}

function openK8sGuide() {
  window.open('https://docs.cloudflow.io/k8s-deployment', '_blank')
}

const newProbe = ref({ name: '', type: 'agent', group: '' })
const newGroup = ref('')
const loading = ref(false)
const savingEdit = ref(false)

// 编辑探针状态
const showEditModal = ref(false)
const editTarget = ref(null)
const editForm = ref({
  name: '',
  group: '',
  newGroup: '',
  maxCpuCore: 2,
  maxMemoryMb: 2048,
  collect: { cpu: false, memory: false, network: false, disk: false },
  ebpf: {
    enabled: false,
    tcpMetrics: { enabled: false },
    httpMetrics: { enabled: false },
    protocolParsing: { enabled: false },
    resourceLimit: { maxCpuCore: 0, maxMemoryMb: 0 }
  }
})

// 分组编辑状态
const editingGroup = ref('')
const editingGroupName = ref('')

let refreshTimer = null

const sshForm = ref({
  host: '',
  port: 22,
  username: 'root',
  authType: 'password',
  password: '',
  privateKey: '',
  probeName: '',
  probeType: 'agent',
  group: '',
  edgeAddr: 'edge:50051'
})

const groups = ref([])
const probes = ref([])
const ebpfStatus = ref(null)

// ========== 数据加载 ==========
async function fetchProbes() {
  loading.value = true
  try {
    const res = await api.getAgents()
    if (res && Array.isArray(res)) {
      probes.value = res
      const groupSet = new Set(res.map(p => p.group).filter(Boolean))
      groups.value = Array.from(groupSet)
    } else {
      probes.value = []
    }
  } catch (e) {
    console.error('获取探针列表失败:', e)
    probes.value = []
  } finally {
    loading.value = false
  }
  
  // 获取 eBPF 探针状态并加入列表
  try {
    const ebpfRes = await fetch('/api/probe/status')
    if (ebpfRes.ok) {
      const ebpfData = await ebpfRes.json()
      ebpfStatus.value = ebpfData
      // 构建 eBPF 探针条目，匹配表格字段
      const ebpfEntry = {
        id: 'ebpf-vm2',
        name: 'eBPF-VM2',
        type: 'eBPF',
        group: '默认',
        version: '1.0.0',
        ip: '192.168.58.131',
        status: ebpfData.status === 'running' ? 'online' : 'offline',
        lastHeartbeat: new Date().toISOString(),
        flows: ebpfData.flows_total || '-',
        uptime: ebpfData.uptime || '',
        recent_logs: ebpfData.recent_logs || ''
      }
      const idx = probes.value.findIndex(p => p.id === 'ebpf-vm2')
      if (idx >= 0) {
        probes.value[idx] = ebpfEntry
      } else {
        probes.value.push(ebpfEntry)
      }
    }
  } catch (e) {
    console.error('获取 eBPF 探针状态失败:', e)
  }
}

onMounted(() => {
  fetchProbes()
  // 每 30 秒自动刷新探针状态
  refreshTimer = setInterval(fetchProbes, 30000)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})

const filteredProbes = computed(() => {
  return probes.value.filter(probe => {
    const typeMatch = selectedType.value === 'all' || probe.type === selectedType.value
    const groupMatch = selectedGroup.value === 'all' || probe.group === selectedGroup.value
    return typeMatch && groupMatch
  })
})

const onlineCount = computed(() => probes.value.filter(p => p.status === 'online').length)
const offlineCount = computed(() => probes.value.filter(p => p.status === 'offline').length)

const canInstall = computed(() => {
  return sshForm.value.host && sshForm.value.username && 
    (sshForm.value.authType === 'key' ? sshForm.value.privateKey : sshForm.value.password) &&
    sshForm.value.probeName
})

function getGroupCount(group) {
  return probes.value.filter(p => p.group === group).length
}

function upgradeProbe(probe) {
  alert(`正在升级探针: ${probe.name} (功能开发中)`)
}

async function uninstallProbe(probe) {
  if (!confirm(`确定要卸载探针 ${probe.name} 吗？`)) return
  try {
    const res = await api.uninstallAgent(probe.id)
    if (res !== null) {
      probes.value = probes.value.filter(p => p.id !== probe.id)
      alert('卸载指令已发送')
    } else {
      alert('卸载失败，请重试')
    }
  } catch (e) {
    alert('卸载失败: ' + e.message)
  }
}

// ========== 编辑探针 ==========
function editProbe(probe) {
  editTarget.value = probe
  const cfg = probe.config || {}
  editForm.value = {
    name: probe.name,
    group: probe.group || '',
    newGroup: '',
    maxCpuCore: cfg.resource_limit?.max_cpu_core ?? 2,
    maxMemoryMb: cfg.resource_limit?.max_memory_mb ?? 2048,
    collect: {
      cpu: cfg.collect?.cpu ?? false,
      memory: cfg.collect?.memory ?? false,
      network: cfg.collect?.network ?? false,
      disk: cfg.collect?.disk ?? false
    },
    ebpf: {
      enabled: cfg.ebpf?.enabled ?? false,
      tcpMetrics: { enabled: cfg.ebpf?.tcp_metrics?.enabled ?? false },
      httpMetrics: { enabled: cfg.ebpf?.http_metrics?.enabled ?? false },
      protocolParsing: { enabled: cfg.ebpf?.protocol_parsing?.enabled ?? false },
      resourceLimit: {
        maxCpuCore: cfg.ebpf?.resource_limit?.max_cpu_core ?? 0,
        maxMemoryMb: cfg.ebpf?.resource_limit?.max_memory_mb ?? 0
      }
    }
  }
  showEditModal.value = true
}

async function saveProbeEdit() {
  if (!editForm.value.name.trim()) { alert('探针名称不能为空'); return }

  savingEdit.value = true

  // 处理新分组
  let finalGroup = editForm.value.group
  if (editForm.value.newGroup.trim()) {
    finalGroup = editForm.value.newGroup.trim()
    if (!groups.value.includes(finalGroup)) {
      groups.value.push(finalGroup)
    }
  }

  const configPayload = {
    name: editForm.value.name.trim(),
    group: finalGroup,
    resource_limit: {
      max_cpu_core: editForm.value.maxCpuCore,
      max_memory_mb: editForm.value.maxMemoryMb
    },
    collect: { ...editForm.value.collect },
    ebpf: {
      enabled: editForm.value.ebpf.enabled,
      tcp_metrics: { enabled: editForm.value.ebpf.tcpMetrics.enabled },
      http_metrics: { enabled: editForm.value.ebpf.httpMetrics.enabled },
      protocol_parsing: { enabled: editForm.value.ebpf.protocolParsing.enabled },
      resource_limit: {
        max_cpu_core: editForm.value.ebpf.resourceLimit.maxCpuCore,
        max_memory_mb: editForm.value.ebpf.resourceLimit.maxMemoryMb
      }
    }
  }

  try {
    const res = await api.updateAgentConfig(editTarget.value.id, configPayload)
    if (res !== null) {
      // 更新本地数据
      Object.assign(editTarget.value, {
        name: configPayload.name,
        group: finalGroup,
        config: configPayload
      })
      showEditModal.value = false
      alert('配置保存成功，探针将在下次心跳同步时应用新配置')
    } else {
      alert('保存失败，请重试')
    }
  } catch (e) {
    alert('保存失败: ' + e.message)
  } finally {
    savingEdit.value = false
  }
}

// ========== 停止 / 重启探针 ==========
async function startProbe(probe) {
  if (probe && probe.type === 'eBPF') {
    try {
      const r = await fetch('/api/probe/start', { method: 'POST' })
      const d = await r.json()
      notify('success', d.success ? 'eBPF 启动成功' : 'eBPF 启动失败')
    } catch(e) { notify('error', '启动失败: ' + e.message) }
    await fetchProbes()
    return
  }
}

async function stopProbe(probe) {
  if (probe.status !== 'online') return
  if (!confirm(`确定要停止探针 ${probe.name} 吗？`)) return
  try {
    const res = await api.updateAgentConfig(probe.id, { action: 'stop' })
    if (res !== null) {
      probe.status = 'offline'
      alert(`探针 ${probe.name} 已发送停止指令`)
    } else {
      alert('操作失败，请重试')
    }
  } catch (e) {
    alert('操作失败: ' + e.message)
  }
}

async function restartProbe(probe) {
  if (probe.status === 'offline') return
  if (!confirm(`确定要重启探针 ${probe.name} 吗？`)) return
  try {
    const res = await api.updateAgentConfig(probe.id, { action: 'restart' })
    if (res !== null) {
      alert(`探针 ${probe.name} 已发送重启指令`)
      // 短暂延迟后刷新状态
      setTimeout(fetchProbes, 3000)
    } else {
      alert('操作失败，请重试')
    }
  } catch (e) {
    alert('操作失败: ' + e.message)
  }
}

async function installProbe() {
  if (!newProbe.value.name) { alert('请输入探针名称'); return }
  try {
    const res = await api.installAgentLocal({
      name: newProbe.value.name,
      type: newProbe.value.type,
      group: newProbe.value.group || '默认'
    })
    if (res !== null) {
      alert(`探针 ${newProbe.value.name} 安装指令已下发`)
      showInstallModal.value = false
      newProbe.value = { name: '', type: 'agent', group: '' }
      // 刷新列表
      fetchProbes()
    } else {
      alert('安装失败，请重试')
    }
  } catch (e) {
    alert('安装失败: ' + e.message)
  }
}

function addGroup() {
  const name = newGroup.value.trim()
  if (name && !groups.value.includes(name)) {
    groups.value.push(name)
    newGroup.value = ''
  }
}

function deleteGroup(group) {
  if (!confirm(`确定删除分组「${group}」吗？该分组下的探针将变为未分组状态。`)) return
  groups.value = groups.value.filter(g => g !== group)
  // 将该分组下探针的 group 清空
  probes.value.forEach(p => { if (p.group === group) p.group = '' })
}

function startRenameGroup(group) {
  editingGroup.value = group
  editingGroupName.value = group
}

async function saveGroupRename(oldName) {
  const newName = editingGroupName.value.trim()
  if (!newName || newName === oldName) { cancelGroupRename(); return }
  if (groups.value.includes(newName)) { alert('分组名称已存在'); return }

  // 更新本地分组名
  const idx = groups.value.indexOf(oldName)
  if (idx >= 0) groups.value[idx] = newName
  // 更新探针的分组
  probes.value.forEach(p => { if (p.group === oldName) p.group = newName })

  // 尝试同步到后端（如果有 API）
  try {
    await api.updateAgentConfig('', { group_rename: { from: oldName, to: newName } })
  } catch (e) { /* 本地更新即可 */ }

  cancelGroupRename()
}

function cancelGroupRename() {
  editingGroup.value = ''
  editingGroupName.value = ''
}

function closeSSHModal() {
  if (installProgress.value === 0 || installProgress.value === 100) {
    showSSHModal.value = false
    resetSSHForm()
  }
}

function resetSSHForm() {
  installProgress.value = 0
  installStatus.value = ''
  installLogs.value = []
  sshForm.value = {
    host: '',
    port: 22,
    username: 'root',
    authType: 'password',
    password: '',
    privateKey: '',
    probeName: '',
    probeType: 'agent',
    group: '',
    edgeAddr: 'edge:50051'
  }
}

async function startSSHInstall() {
  if (!canInstall.value) return

  installing.value = true
  installProgress.value = 0
  installLogs.value = []

  const log = (msg) => { installLogs.value.push(msg) }

  try {
    log(`[SSH] 正在连接 ${sshForm.value.host}:${sshForm.value.port}...`)
    installProgress.value = 10
    await new Promise(r => setTimeout(r, 500))

    log('[API] 发送 SSH 安装请求到 agent-manager...')
    installProgress.value = 30
    await new Promise(r => setTimeout(r, 500))

    const res = await api.installAgentSSH({
      host: sshForm.value.host,
      port: sshForm.value.port,
      username: sshForm.value.username,
      auth_type: sshForm.value.authType,
      password: sshForm.value.password,
      private_key: sshForm.value.privateKey,
      probe_name: sshForm.value.probeName,
      probe_type: sshForm.value.probeType,
      group: sshForm.value.group || '默认',
      edge_addr: sshForm.value.edgeAddr
    })

    if (res !== null) {
      installProgress.value = 80
      log(`[SUCCESS] 探针 ${sshForm.value.probeName} 安装指令已下发`)
      await new Promise(r => setTimeout(r, 500))
      installProgress.value = 100
      installStatus.value = '安装完成！'

      // 刷新列表
      fetchProbes()

      setTimeout(() => { if (showSSHModal.value) closeSSHModal() }, 2000)
    } else {
      throw new Error('后端返回空响应')
    }
  } catch (e) {
    installProgress.value = 0
    log(`[ERROR] ${e.message}`)
    installStatus.value = '安装失败'
  } finally {
    installing.value = false
  }
}



async function controlEBPF(action) {
  try {
    const res = await fetch('/api/probe/' + action, { method: 'POST' })
    if (res.ok) {
      const data = await res.json()
      notify('success', 'eBPF ' + (action === 'start' ? '启动' : action === 'stop' ? '停止' : '重启') + '成功')
    } else {
      notify('error', '操作失败')
    }
  } catch (e) {
    notify('error', '操作异常: ' + e.message)
  } finally {
    await fetchEBPFStatus()
  }
}

// Also call fetchEBPFStatus in onMounted

</script>

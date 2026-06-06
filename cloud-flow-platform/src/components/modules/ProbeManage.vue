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
          <Kubernetes class="w-4 h-4" />
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
            <td class="px-4 py-3">
              <div class="flex gap-2">
                <button @click="upgradeProbe(probe)" class="p-1.5 hover:bg-dark-600 rounded text-primary-400" title="升级">
                  <ArrowUpCircle class="w-4 h-4" />
                </button>
                <button @click="uninstallProbe(probe)" class="p-1.5 hover:bg-dark-600 rounded text-red-400" title="卸载">
                  <Trash2 class="w-4 h-4" />
                </button>
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
              <Kubernetes class="w-4 h-4" />
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
        <div class="space-y-2">
          <div v-for="group in groups" :key="group" class="flex items-center justify-between p-3 bg-dark-700 rounded-lg">
            <span class="text-white">{{ group }}</span>
            <span class="text-sm text-gray-400">{{ getGroupCount(group) }} 个探针</span>
          </div>
        </div>
        <div class="mt-4 flex gap-2">
          <input v-model="newGroup" type="text" class="flex-1 px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:border-primary-500" placeholder="新建分组名称" />
          <button @click="addGroup" class="px-4 py-2 bg-primary-500 text-white rounded-lg hover:bg-primary-600 transition">添加</button>
        </div>
        <div class="flex justify-end mt-6">
          <button @click="showGroupModal = false" class="px-4 py-2 bg-primary-500 text-white rounded-lg font-medium hover:bg-primary-600 transition">关闭</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
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
  Kubernetes,
  Copy,
  FileCode,
  HelpCircle,
  AlertCircle,
  Database
} from 'lucide-vue-next'

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

const groups = ['华北', '华东', '华南', '中心', '边缘']

const probes = ref([
  { id: 1, name: 'agent-prod-01', type: 'agent', group: '中心', version: 'v1.2.3', ip: '192.168.1.101', status: 'online', lastHeartbeat: '2分钟前' },
  { id: 2, name: 'agent-prod-02', type: 'agent', group: '中心', version: 'v1.2.3', ip: '192.168.1.102', status: 'online', lastHeartbeat: '1分钟前' },
  { id: 3, name: 'center-main', type: 'center', group: '中心', version: 'v1.2.3', ip: '192.168.1.10', status: 'online', lastHeartbeat: '30秒前' },
  { id: 4, name: 'edge-beijing', type: 'edge', group: '华北', version: 'v1.2.2', ip: '10.0.1.10', status: 'online', lastHeartbeat: '5分钟前' },
  { id: 5, name: 'edge-shanghai', type: 'edge', group: '华东', version: 'v1.2.3', ip: '10.0.2.10', status: 'offline', lastHeartbeat: '15分钟前' },
  { id: 6, name: 'edge-guangzhou', type: 'edge', group: '华南', version: 'v1.2.3', ip: '10.0.3.10', status: 'online', lastHeartbeat: '1分钟前' }
])

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
  alert(`正在升级探针: ${probe.name}`)
}

function uninstallProbe(probe) {
  if (confirm(`确定要卸载探针 ${probe.name} 吗？`)) {
    probes.value = probes.value.filter(p => p.id !== probe.id)
  }
}

function installProbe() {
  const newId = Date.now()
  probes.value.push({
    id: newId,
    name: newProbe.value.name,
    type: newProbe.value.type,
    group: newProbe.value.group || '默认',
    version: 'v1.2.3',
    ip: '待分配',
    status: 'online',
    lastHeartbeat: '刚刚'
  })
  showInstallModal.value = false
  newProbe.value = { name: '', type: 'agent', group: '' }
}

function addGroup() {
  if (newGroup.value && !groups.includes(newGroup.value)) {
    groups.push(newGroup.value)
    newGroup.value = ''
  }
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
  
  // 模拟安装进度
  const steps = [
    { progress: 10, status: '正在连接远程服务器...', log: `[SSH] Connecting to ${sshForm.value.host}:${sshForm.value.port}...` },
    { progress: 25, status: 'SSH 连接成功', log: '[SSH] Connection established successfully' },
    { progress: 35, status: '正在验证凭据...', log: '[AUTH] Authenticating with username/password...' },
    { progress: 45, status: '凭据验证通过', log: '[AUTH] Authentication successful' },
    { progress: 55, status: '正在下载安装脚本...', log: '[DOWNLOAD] Fetching install script from https://install.cloudflow.io/probe.sh...' },
    { progress: 65, status: '正在安装探针...', log: '[INSTALL] Running install.sh with options:' },
    { progress: 75, status: '安装中...', log: `[INSTALL] --name=${sshForm.value.probeName} --type=${sshForm.value.probeType}` },
    { progress: 85, status: '正在配置服务...', log: '[CONFIG] Creating systemd service...' },
    { progress: 95, status: '正在启动服务...', log: '[START] Starting cloud-flow-agent service...' },
    { progress: 100, status: '安装完成！', log: '[SUCCESS] cloud-flow-agent installed and running' }
  ]
  
  for (const step of steps) {
    await new Promise(resolve => setTimeout(resolve, 800))
    installProgress.value = step.progress
    installStatus.value = step.status
    installLogs.value.push(step.log)
  }
  
  // 添加新探针到列表
  probes.value.push({
    id: Date.now(),
    name: sshForm.value.probeName,
    type: sshForm.value.probeType,
    group: sshForm.value.group || '默认',
    version: 'v1.2.3',
    ip: sshForm.value.host,
    status: 'online',
    lastHeartbeat: '刚刚'
  })
  
  installing.value = false
  
  // 3秒后自动关闭
  setTimeout(() => {
    if (showSSHModal.value) {
      closeSSHModal()
    }
  }, 3000)
}
</script>

<template>
  <aside class="fixed left-0 top-0 bottom-0 w-64 bg-dark-800 border-r border-dark-600 flex flex-col z-40">
    <div class="p-4 border-b border-dark-600">
      <div class="flex items-center gap-3">
        <div class="w-10 h-10 bg-gradient-to-br from-primary-500 to-primary-700 rounded-lg flex items-center justify-center">
          <Activity class="w-6 h-6 text-white" />
        </div>
        <div>
          <h1 class="text-lg font-bold text-white">CloudFlow</h1>
          <p class="text-xs text-gray-400">Network Analytics</p>
        </div>
      </div>
    </div>
    
    <nav class="flex-1 p-4">
      <ul class="space-y-2">
        <li v-for="item in menuItems" :key="item.id">
          <button
            @click="$emit('change', item.id)"
            :class="[
              'w-full flex items-center gap-3 px-4 py-3 rounded-lg transition-all duration-200',
              activeModule === item.id
                ? 'bg-primary-500/20 text-primary-400 border border-primary-500/30'
                : 'text-gray-400 hover:bg-dark-700 hover:text-white'
            ]"
          >
            <component :is="item.icon" class="w-5 h-5" />
            <span class="text-sm font-medium">{{ item.label }}</span>
            <span 
              v-if="item.badge" 
              class="ml-auto text-xs px-2 py-0.5 rounded-full"
              :class="item.badgeClass"
            >
              {{ item.badge }}
            </span>
          </button>
        </li>
      </ul>
    </nav>
    
    <div class="p-4 border-t border-dark-600">
      <div class="flex items-center gap-3">
        <div class="w-9 h-9 bg-dark-700 rounded-full flex items-center justify-center">
          <User class="w-5 h-5 text-gray-400" />
        </div>
        <div class="flex-1 min-w-0">
          <p class="text-sm text-white truncate">Admin</p>
          <p class="text-xs text-gray-500">Administrator</p>
        </div>
        <button class="p-2 hover:bg-dark-700 rounded-lg transition">
          <Settings class="w-4 h-4 text-gray-400" />
        </button>
      </div>
    </div>
  </aside>
</template>

<script setup>
import { Activity, LayoutDashboard, TrendingUp, Network, AlertTriangle, Server, Download, User, Settings } from 'lucide-vue-next'

defineProps({
  activeModule: {
    type: String,
    default: 'overview'
  }
})

defineEmits(['change'])

const menuItems = [
  { id: 'overview', label: '总览', icon: LayoutDashboard },
  { id: 'traffic', label: '流量监控', icon: TrendingUp },
  { id: 'network', label: '网络分析', icon: Network },
  { id: 'alerts', label: '告警中心', icon: AlertTriangle, badge: '3', badgeClass: 'bg-red-500/20 text-red-400' },
  { id: 'k8s', label: 'K8s 资源', icon: Server },
  { id: 'export', label: '数据导出', icon: Download }
]
</script>

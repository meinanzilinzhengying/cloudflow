<template>
  <aside
    :class="[
      'fixed left-0 top-0 bottom-0 z-40 flex flex-col',
      'bg-slate-900 dark:bg-dark-950',
      'border-r border-slate-800 dark:border-dark-800',
      'transition-all duration-300 ease-in-out',
      collapsed ? 'w-[72px]' : 'w-[240px]'
    ]"
  >
    <!-- Logo -->
    <div class="h-16 flex items-center px-4 border-b border-slate-800 dark:border-dark-800">
      <div class="flex items-center gap-3 overflow-hidden">
        <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-primary-500 to-accent-500 flex items-center justify-center flex-shrink-0 shadow-lg shadow-primary-500/25">
          <svg class="w-6 h-6 text-white" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 2L2 7l10 5 10-5-10-5z"/>
            <path d="M2 17l10 5 10-5"/>
            <path d="M2 12l10 5 10-5"/>
          </svg>
        </div>
        <div v-if="!collapsed" class="animate-fade-in">
          <h1 class="text-lg font-bold text-white tracking-tight">CloudFlow</h1>
          <p class="text-[10px] text-slate-500 uppercase tracking-widest">Observability</p>
        </div>
      </div>
    </div>

    <!-- Navigation -->
    <nav class="flex-1 overflow-y-auto py-4 px-3 scrollbar-hide">
      <!-- Main Menu -->
      <div class="space-y-1">
        <div v-for="item in mainMenuItems" :key="item.id">
          <button
            @click="handleMenuClick(item.id)"
            :class="[
              'w-full flex items-center gap-3 px-3 py-2.5 rounded-xl',
              'transition-all duration-200 group relative',
              activeMenu === item.id
                ? 'bg-primary-500/15 text-primary-400'
                : 'text-slate-400 hover:bg-slate-800 hover:text-white'
            ]"
          >
            <div
              v-if="activeMenu === item.id"
              class="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-6 bg-primary-500 rounded-r-full"
            ></div>
            <component :is="item.icon" class="w-5 h-5 flex-shrink-0" />
            <span v-if="!collapsed" class="text-sm font-medium truncate">{{ item.label }}</span>
            <span
              v-if="item.badge && !collapsed"
              :class="[
                'ml-auto text-xs px-2 py-0.5 rounded-full font-medium',
                item.badgeType === 'danger' ? 'bg-red-500/20 text-red-400' : 'bg-primary-500/20 text-primary-400'
              ]"
            >
              {{ item.badge }}
            </span>
            <div
              v-if="collapsed"
              class="absolute left-full ml-2 px-2 py-1 bg-slate-800 text-white text-xs rounded opacity-0 invisible group-hover:opacity-100 group-hover:visible transition-all whitespace-nowrap z-50"
            >
              {{ item.label }}
            </div>
          </button>
        </div>
      </div>

      <div class="my-4 h-px bg-gradient-to-r from-transparent via-slate-700 to-transparent"></div>

      <!-- Management Menu -->
      <div class="space-y-1">
        <p v-if="!collapsed" class="px-3 py-2 text-[10px] font-semibold text-slate-600 uppercase tracking-wider">
          管理中心
        </p>
        <div v-for="item in managementMenuItems" :key="item.id">
          <button
            @click="handleMenuClick(item.id)"
            :class="[
              'w-full flex items-center gap-3 px-3 py-2.5 rounded-xl',
              'transition-all duration-200 group relative',
              activeMenu === item.id
                ? 'bg-primary-500/15 text-primary-400'
                : 'text-slate-400 hover:bg-slate-800 hover:text-white'
            ]"
          >
            <div
              v-if="activeMenu === item.id"
              class="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-6 bg-primary-500 rounded-r-full"
            ></div>
            <component :is="item.icon" class="w-5 h-5 flex-shrink-0" />
            <span v-if="!collapsed" class="text-sm font-medium truncate">{{ item.label }}</span>
            <div
              v-if="collapsed"
              class="absolute left-full ml-2 px-2 py-1 bg-slate-800 text-white text-xs rounded opacity-0 invisible group-hover:opacity-100 group-hover:visible transition-all whitespace-nowrap z-50"
            >
              {{ item.label }}
            </div>
          </button>
        </div>
      </div>
    </nav>

    <!-- Footer -->
    <div class="p-4 border-t border-slate-800 dark:border-dark-800">
      <button
        @click="$emit('toggle')"
        class="w-full flex items-center justify-center gap-2 px-3 py-2 rounded-xl text-slate-500 hover:bg-slate-800 hover:text-white transition-all duration-200"
      >
        <component :is="collapsed ? ChevronRight : ChevronLeft" class="w-5 h-5" />
        <span v-if="!collapsed" class="text-xs">收起菜单</span>
      </button>
    </div>
  </aside>
</template>

<script setup>
import {
  LayoutDashboard,
  TrendingUp,
  GitBranch,
  Activity,
  BarChart3,
  FileText,
  AlertTriangle,
  Building2,
  Users,
  Server,
  Settings,
  ChevronLeft,
  ChevronRight,
} from 'lucide-vue-next'

defineProps({
  collapsed: { type: Boolean, default: false },
  activeMenu: { type: String, default: 'dashboard' },
  activeSubmenu: { type: String, default: null },
})

const emit = defineEmits(['toggle', 'menu-change'])

const mainMenuItems = [
  { id: 'dashboard', label: '仪表盘', icon: LayoutDashboard },
  { id: 'traffic', label: '流量分析', icon: TrendingUp },
  { id: 'topology', label: '拓扑可视化', icon: GitBranch },
  { id: 'tracing', label: '链路追踪', icon: Activity },
  { id: 'metrics', label: '指标监控', icon: BarChart3 },
  { id: 'logs', label: '日志分析', icon: FileText },
  { id: 'alerts', label: '告警中心', icon: AlertTriangle, badge: '3', badgeType: 'danger' },
]

const managementMenuItems = [
  { id: 'tenants', label: '租户管理', icon: Building2 },
  { id: 'users', label: '用户管理', icon: Users },
  { id: 'agents', label: 'Agent 管理', icon: Server },
  { id: 'settings', label: '系统设置', icon: Settings },
]

const handleMenuClick = (id) => {
  emit('menu-change', id)
}
</script>

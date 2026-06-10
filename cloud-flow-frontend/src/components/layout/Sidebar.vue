<template>
  <aside
    :class="[
      'fixed left-0 top-0 z-40 flex flex-col h-screen',
      'bg-slate-900 dark:bg-dark-950',
      'border-r border-slate-800 dark:border-dark-800',
      'transition-all duration-300 ease-in-out overflow-hidden',
      collapsed ? 'w-[72px]' : 'w-[260px]'
    ]"
  >
    <!-- Logo -->
    <div class="h-16 flex-shrink-0 flex items-center px-4 border-b border-slate-800 dark:border-dark-800">
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
    <nav class="flex-1 min-h-0 overflow-y-auto py-4 px-3 sidebar-nav-scroll">
      <div v-for="menu in menuItems" :key="menu.id" class="mb-1">
        <!-- Parent Menu -->
        <div class="relative">
          <button
            @click="toggleSubmenu(menu.id)"
            :class="[
              'w-full flex items-center gap-3 px-3 py-2.5 rounded-xl',
              'transition-all duration-200 group relative',
              isActiveMenu(menu.id)
                ? 'bg-primary-500/15 text-primary-400'
                : 'text-slate-400 hover:bg-slate-800 hover:text-white'
            ]"
          >
            <!-- Active indicator -->
            <div
              v-if="isActiveMenu(menu.id)"
              class="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-6 bg-primary-500 rounded-r-full"
            ></div>

            <component :is="menu.icon" class="w-5 h-5 flex-shrink-0" />
            <span v-if="!collapsed" class="text-sm font-medium truncate flex-1 text-left">{{ menu.label }}</span>
            
            <!-- Badge -->
            <span
              v-if="menu.badge && !collapsed"
              :class="[
                'text-xs px-2 py-0.5 rounded-full font-medium',
                menu.badgeType === 'danger' ? 'bg-red-500/20 text-red-400' : 'bg-primary-500/20 text-primary-400'
              ]"
            >
              {{ menu.badge }}
            </span>

            <!-- Chevron for submenu -->
            <ChevronRight
              v-if="menu.children && !collapsed"
              :class="[
                'w-4 h-4 transition-transform duration-200',
                expandedMenus.includes(menu.id) ? 'rotate-90' : ''
              ]"
            />

            <!-- Tooltip for collapsed state -->
            <div
              v-if="collapsed"
              class="absolute left-full ml-2 px-2 py-1 bg-slate-800 text-white text-xs rounded opacity-0 invisible group-hover:opacity-100 group-hover:visible transition-all whitespace-nowrap z-50"
            >
              {{ menu.label }}
            </div>
          </button>

          <!-- Submenu -->
          <Transition name="submenu">
            <div v-if="menu.children && expandedMenus.includes(menu.id) && !collapsed" class="mt-1 ml-3">
              <div class="space-y-0.5">
                <button
                  v-for="child in menu.children"
                  :key="child.id"
                  @click="handleMenuClick(child.id)"
                  :class="[
                    'w-full flex items-center gap-3 px-3 py-2 rounded-lg',
                    'transition-all duration-200',
                    activeMenu === child.id
                      ? 'bg-primary-500/10 text-primary-400'
                      : 'text-slate-400 hover:bg-slate-800 hover:text-white'
                  ]"
                >
                  <component :is="child.icon || Circle" class="w-3.5 h-3.5 flex-shrink-0" />
                  <span class="text-sm truncate">{{ child.label }}</span>
                </button>
              </div>
            </div>
          </Transition>
        </div>
      </div>
    </nav>

    <!-- Footer -->
    <div class="flex-shrink-0 p-4 border-t border-slate-800 dark:border-dark-800">
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
import { ref, computed } from 'vue'
import {
  LayoutDashboard,
  TrendingUp,
  GitBranch,
  Activity,
  BarChart3,
  FileText,
  AlertTriangle,
  Search,
  RefreshCw,
  Circle,
  ChevronLeft,
  ChevronRight,
  Building2,
  Users,
  Server,
  Settings,
  Key,
  Database,
  Layers,
  Clock,
  Zap,
  Code,
  Network,
  Globe,
  History,
  Bug,
  Lightbulb,
  Map,
  Bell,
} from 'lucide-vue-next'

const props = defineProps({
  collapsed: { type: Boolean, default: false },
  activeMenu: { type: String, default: 'dashboard' },
})

const emit = defineEmits(['toggle', 'menu-change'])

const expandedMenus = ref(['traffic', 'topology', 'tracing', 'metrics', 'logs', 'alerts', 'rca', 'management'])

const menuItems = computed(() => [
  {
    id: 'dashboard',
    label: '仪表盘',
    icon: LayoutDashboard,
  },
  {
    id: 'traffic',
    label: '网络流量',
    icon: TrendingUp,
    children: [
      { id: 'traffic-overview', label: '全局流量分析', icon: Globe },
      { id: 'traffic-sessions', label: '会话分析', icon: Network },
      { id: 'traffic-topn', label: 'TopN排行', icon: BarChart3 },
      { id: 'traffic-map', label: '流量地图', icon: Map },
      { id: 'traffic-replay', label: '流量回放', icon: History },
    ],
  },
  {
    id: 'topology',
    label: '服务拓扑',
    icon: GitBranch,
    children: [
      { id: 'topology-service', label: '服务地图', icon: Layers },
      { id: 'topology-pod', label: 'Pod拓扑', icon: Database },
      { id: 'topology-process', label: '进程拓扑', icon: Code },
      { id: 'topology-namespace', label: 'Namespace拓扑', icon: Building2 },
      { id: 'topology-diff', label: '拓扑变更对比', icon: GitBranch },
    ],
  },
  {
    id: 'tracing',
    label: '链路追踪',
    icon: Activity,
    children: [
      { id: 'tracing-query', label: 'Trace查询', icon: Search },
      { id: 'tracing-slow', label: '慢请求分析', icon: Clock },
      { id: 'tracing-errors', label: '错误请求分析', icon: Bug },
      { id: 'tracing-calls', label: '服务调用分析', icon: Zap },
    ],
  },
  {
    id: 'metrics',
    label: '指标监控',
    icon: BarChart3,
    children: [
      { id: 'metrics-host', label: '主机指标', icon: Server },
      { id: 'metrics-container', label: '容器指标', icon: Database },
      { id: 'metrics-service', label: '服务指标', icon: Layers },
      { id: 'metrics-custom', label: '自定义指标', icon: Settings },
    ],
  },
  {
    id: 'logs',
    label: '日志分析',
    icon: FileText,
    children: [
      { id: 'logs-search', label: '日志检索', icon: Search },
      { id: 'logs-aggregate', label: '日志聚合', icon: BarChart3 },
      { id: 'logs-correlation', label: '日志关联分析', icon: Network },
    ],
  },
  {
    id: 'alerts',
    label: '告警中心',
    icon: AlertTriangle,
    badge: '3',
    badgeType: 'danger',
    children: [
      { id: 'alerts-events', label: '告警事件', icon: AlertTriangle },
      { id: 'alerts-rules', label: '告警规则', icon: Settings },
      { id: 'alerts-notifications', label: '通知策略', icon: Bell },
      { id: 'alerts-stats', label: '告警统计', icon: BarChart3 },
    ],
  },
  {
    id: 'rca',
    label: '根因分析',
    icon: Lightbulb,
    children: [
      { id: 'rca-analysis', label: 'RCA分析', icon: Lightbulb },
      { id: 'rca-correlation', label: '事件关联', icon: Network },
      { id: 'rca-timeline', label: '故障时间线', icon: History },
    ],
  },
  {
    id: 'management',
    label: '管理中心',
    icon: Settings,
    children: [
      { id: 'management-agents', label: 'Agent管理', icon: Server },
      { id: 'management-users', label: '用户管理', icon: Users },
      { id: 'management-tenants', label: '租户管理', icon: Building2 },
      { id: 'management-apikey', label: 'API Key', icon: Key },
      { id: 'management-settings', label: '系统设置', icon: Settings },
    ],
  },
])

const toggleSubmenu = (menuId) => {
  const index = expandedMenus.value.indexOf(menuId)
  const menu = menuItems.value.find((m) => m.id === menuId)
  
  if (index > -1) {
    expandedMenus.value.splice(index, 1)
  } else {
    expandedMenus.value.push(menuId)
    if (menu?.children && menu.children.length > 0) {
      emit('menu-change', menu.children[0].id)
    }
  }
}

const isActiveMenu = (menuId) => {
  const menu = menuItems.value.find((m) => m.id === menuId)
  if (menu?.children) {
    return menu.children.some((child) => child.id === props.activeMenu)
  }
  return props.activeMenu === menuId
}

const handleMenuClick = (menuId) => {
  emit('menu-change', menuId)
}
</script>

<style scoped>
.sidebar-nav-scroll {
  scrollbar-width: thin;
  scrollbar-color: rgba(148, 163, 184, 0.3) transparent;
}

.sidebar-nav-scroll::-webkit-scrollbar {
  width: 6px;
}

.sidebar-nav-scroll::-webkit-scrollbar-track {
  background: transparent;
}

.sidebar-nav-scroll::-webkit-scrollbar-thumb {
  background: rgba(148, 163, 184, 0.3);
  border-radius: 3px;
}

.sidebar-nav-scroll::-webkit-scrollbar-thumb:hover {
  background: rgba(148, 163, 184, 0.5);
}

.submenu-enter-active,
.submenu-leave-active {
  transition: all 0.2s ease;
  overflow: hidden;
}

.submenu-enter-from,
.submenu-leave-to {
  opacity: 0;
  transform: translateY(-8px);
  max-height: 0;
}

.submenu-enter-to,
.submenu-leave-from {
  max-height: 500px;
}
</style>

<template>
  <header class="h-16 flex items-center justify-between px-6 border-b border-slate-200 dark:border-dark-700 bg-white/80 dark:bg-dark-800/80 backdrop-blur-xl sticky top-0 z-30">
    <!-- Left Section -->
    <div class="flex items-center gap-4">
      <div class="flex items-center gap-3 px-3 py-1.5 rounded-lg bg-slate-100 dark:bg-dark-700">
        <div class="w-2 h-2 rounded-full bg-green-500 animate-pulse"></div>
        <span class="text-sm font-medium text-slate-700 dark:text-slate-200">生产环境</span>
        <span class="text-xs text-slate-500 dark:text-slate-400">|</span>
        <span class="text-sm text-slate-600 dark:text-slate-300">default-tenant</span>
      </div>
    </div>

    <!-- Center Section: Search -->
    <div class="flex-1 max-w-xl mx-8">
      <div class="relative group">
        <Search class="absolute left-4 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400 group-focus-within:text-primary-500 transition-colors" />
        <input
          type="text"
          placeholder="搜索服务、Agent、告警..."
          class="w-full pl-11 pr-4 py-2.5 bg-slate-100 dark:bg-dark-700 border border-transparent rounded-xl text-sm placeholder:text-slate-400 focus:outline-none focus:border-primary-500/50 focus:bg-white dark:focus:bg-dark-600 transition-all"
        />
        <div class="absolute right-3 top-1/2 -translate-y-1/2 flex items-center gap-1">
          <kbd class="hidden sm:inline-flex px-1.5 py-0.5 text-[10px] font-mono text-slate-400 bg-slate-200 dark:bg-dark-600 rounded">⌘K</kbd>
        </div>
      </div>
    </div>

    <!-- Right Section -->
    <div class="flex items-center gap-2">
      <!-- Time Range -->
      <div class="flex items-center gap-1 p-1 bg-slate-100 dark:bg-dark-700 rounded-xl">
        <button
          v-for="range in timeRanges"
          :key="range.value"
          @click="selectedRange = range.value"
          :class="[
            'px-3 py-1.5 text-xs font-medium rounded-lg transition-all duration-200',
            selectedRange === range.value
              ? 'bg-white dark:bg-dark-600 text-slate-900 dark:text-white shadow-sm'
              : 'text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200'
          ]"
        >
          {{ range.label }}
        </button>
      </div>

      <div class="w-px h-6 bg-slate-200 dark:bg-dark-700 mx-2"></div>

      <!-- Actions -->
      <div class="flex items-center gap-1">
        <button @click="$emit('refresh')" class="p-2 rounded-lg text-slate-400 hover:text-slate-600 hover:bg-slate-100 dark:hover:text-white dark:hover:bg-dark-700 transition-all" title="刷新">
          <RefreshCw :class="['w-5 h-5']" />
        </button>

        <button @click="toggleFullscreen" class="p-2 rounded-lg text-slate-400 hover:text-slate-600 hover:bg-slate-100 dark:hover:text-white dark:hover:bg-dark-700 transition-all" title="全屏">
          <Maximize2 v-if="!isFullscreen" class="w-5 h-5" />
          <Minimize2 v-else class="w-5 h-5" />
        </button>

        <button @click="$emit('toggle-theme')" class="p-2 rounded-lg text-slate-400 hover:text-slate-600 hover:bg-slate-100 dark:hover:text-white dark:hover:bg-dark-700 transition-all" :title="isDark ? '亮色模式' : '暗色模式'">
          <Moon v-if="isDark" class="w-5 h-5" />
          <Sun v-else class="w-5 h-5" />
        </button>

        <button class="relative p-2 rounded-lg text-slate-400 hover:text-slate-600 hover:bg-slate-100 dark:hover:text-white dark:hover:bg-dark-700 transition-all">
          <Bell class="w-5 h-5" />
          <span class="absolute top-1 right-1 w-2 h-2 bg-red-500 rounded-full animate-pulse"></span>
        </button>

        <!-- User -->
        <div class="relative ml-2">
          <button @click="showUserMenu = !showUserMenu" class="flex items-center gap-3 p-1.5 rounded-xl hover:bg-slate-100 dark:hover:bg-dark-700 transition-all">
            <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-primary-500 to-accent-500 flex items-center justify-center text-white font-medium text-sm shadow-lg shadow-primary-500/25">
              A
            </div>
            <div class="hidden md:block text-left">
              <p class="text-sm font-medium text-slate-700 dark:text-white">Admin</p>
              <p class="text-xs text-slate-400">管理员</p>
            </div>
            <ChevronDown class="w-4 h-4 text-slate-400" />
          </button>

          <Transition name="dropdown">
            <div v-if="showUserMenu" class="absolute right-0 mt-2 w-56 bg-white dark:bg-dark-800 rounded-xl shadow-xl border border-slate-200 dark:border-dark-700 py-2 z-50">
              <div class="px-4 py-3 border-b border-slate-100 dark:border-dark-700">
                <p class="text-sm font-medium text-slate-900 dark:text-white">Admin</p>
                <p class="text-xs text-slate-500">admin@cloudflow.io</p>
              </div>
              <div class="py-1">
                <a href="#" class="flex items-center gap-3 px-4 py-2 text-sm text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-dark-700">
                  <User class="w-4 h-4" /> 个人设置
                </a>
                <a href="#" class="flex items-center gap-3 px-4 py-2 text-sm text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-dark-700">
                  <Settings class="w-4 h-4" /> 系统设置
                </a>
              </div>
              <div class="border-t border-slate-100 dark:border-dark-700 pt-1">
                <a href="#" class="flex items-center gap-3 px-4 py-2 text-sm text-red-600 hover:bg-red-50 dark:hover:bg-red-500/10">
                  <LogOut class="w-4 h-4" /> 退出登录
                </a>
              </div>
            </div>
          </Transition>
        </div>
      </div>
    </div>
  </header>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import {
  Search,
  RefreshCw,
  Maximize2,
  Minimize2,
  Sun,
  Moon,
  Bell,
  ChevronDown,
  User,
  Settings,
  LogOut,
} from 'lucide-vue-next'

defineProps({
  isDark: { type: Boolean, default: false },
})

const emit = defineEmits(['toggle-theme', 'refresh'])

const isFullscreen = ref(false)
const selectedRange = ref('1h')
const showUserMenu = ref(false)

const timeRanges = [
  { label: '5m', value: '5m' },
  { label: '15m', value: '15m' },
  { label: '1h', value: '1h' },
  { label: '6h', value: '6h' },
  { label: '24h', value: '24h' },
  { label: '7d', value: '7d' },
]

const toggleFullscreen = () => {
  if (document.fullscreenElement) {
    document.exitFullscreen()
    isFullscreen.value = false
  } else {
    document.documentElement.requestFullscreen()
    isFullscreen.value = true
  }
}

const handleClickOutside = (e) => {
  if (!e.target.closest('.relative')) {
    showUserMenu.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.2s ease;
}
.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>

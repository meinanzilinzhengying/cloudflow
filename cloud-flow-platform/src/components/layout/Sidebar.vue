<template>
  <aside class="w-64 bg-dark-800 border-r border-dark-600 flex flex-col">
    <div class="p-4 border-b border-dark-600">
      <div class="flex items-center gap-3">
        <div class="w-10 h-10 bg-gradient-to-br from-primary-500 to-primary-700 rounded-lg flex items-center justify-center">
          <Activity class="w-6 h-6 text-white" />
        </div>
        <div>
          <h1 class="text-lg font-bold text-white">CloudFlow</h1>
          <p class="text-xs text-gray-400">Platform Monitor</p>
        </div>
      </div>
    </div>
    <div class="p-4 flex-1 overflow-y-auto">
      <nav class="space-y-1">
        <button
          v-for="mod in modules"
          :key="mod.key"
          @click="$emit('select', mod.key)"
          :class="[
            'w-full flex items-center gap-3 px-4 py-2.5 rounded-lg transition-all duration-200 text-left',
            activeModule === mod.key 
              ? 'bg-primary-500/20 text-primary-400 border border-primary-500/30' 
              : 'text-gray-400 hover:bg-dark-700 hover:text-white'
          ]"
        >
          <component :is="getIcon(mod.icon)" class="w-5 h-5" />
          <span class="text-sm font-medium">{{ mod.label }}</span>
        </button>
      </nav>
    </div>
    <div class="p-4 border-t border-dark-600">
      <div class="text-xs text-gray-500">v1.0.0</div>
    </div>
  </aside>
</template>

<script setup>
import { Activity, LayoutDashboard, Cpu, HeartPulse, Settings, Bell, Circle } from 'lucide-vue-next'

defineProps({
  modules: {
    type: Array,
    required: true
  },
  activeModule: {
    type: String,
    default: 'dashboard'
  }
})

defineEmits(['select'])

function getIcon(name) {
  const icons = {
    LayoutDashboard,
    Cpu,
    HeartPulse,
    Settings,
    Bell,
    Circle
  }
  return icons[name] || Circle
}
</script>

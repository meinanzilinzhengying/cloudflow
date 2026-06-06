<template>
  <aside class="w-56 bg-dark-200 border-r border-dark-100 flex flex-col">
    <div class="p-4 flex-1">
      <nav class="space-y-1">
        <button
          v-for="mod in modules"
          :key="mod.key"
          @click="$emit('select', mod.key)"
          :class="[
            'w-full flex items-center gap-3 px-3 py-2.5 rounded-lg transition-colors text-left',
            activeModule === mod.key 
              ? 'bg-accent-500/10 text-accent-500' 
              : 'text-gray-400 hover:bg-dark-100 hover:text-white'
          ]"
        >
          <component :is="getIcon(mod.icon)" class="w-5 h-5" />
          <span class="text-sm font-medium">{{ mod.label }}</span>
        </button>
      </nav>
    </div>
    <div class="p-4 border-t border-dark-100">
      <div class="text-xs text-gray-500">v1.0.0</div>
    </div>
  </aside>
</template>

<script setup>
import { LayoutDashboard, Cpu, HeartPulse, Settings, Bell, Circle } from 'lucide-vue-next'

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

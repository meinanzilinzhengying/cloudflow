<template>
  <div class="space-y-6">
    <!-- Page Header -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">租户管理</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">管理租户和项目</p>
      </div>
      <button class="btn-primary">
        <Plus class="w-4 h-4" />
        添加租户
      </button>
    </div>

    <!-- Tenant List -->
    <div class="card">
      <div class="grid grid-cols-3 gap-4">
        <div
          v-for="tenant in tenants"
          :key="tenant.id"
          @click="selectTenant(tenant)"
          :class="[
            'p-4 rounded-xl cursor-pointer transition-all',
            selectedTenant?.id === tenant.id ? 'bg-primary-50 dark:bg-primary-500/10 border-2 border-primary-500' : 'bg-slate-50 dark:bg-dark-700 border-2 border-transparent hover:border-slate-200 dark:hover:border-dark-600'
          ]"
        >
          <div class="flex items-center gap-3 mb-3">
            <div class="w-10 h-10 rounded-xl bg-primary-100 dark:bg-primary-500/20 flex items-center justify-center">
              <Building2 class="w-5 h-5 text-primary-500" />
            </div>
            <div>
              <h3 class="font-semibold text-slate-900 dark:text-white">{{ tenant.name }}</h3>
              <p class="text-xs text-slate-500">{{ tenant.projects.length }} 个项目</p>
            </div>
          </div>
          <div class="flex items-center gap-4 text-xs text-slate-500">
            <span>{{ tenant.agents }} Agents</span>
            <span>{{ tenant.storage }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Tenant Detail Drawer -->
    <Transition name="drawer">
      <div
        v-if="selectedTenant"
        class="fixed inset-0 z-50 flex justify-end"
        @click.self="selectedTenant = null"
      >
        <div class="absolute inset-0 bg-slate-900/50"></div>
        <div class="relative w-full max-w-md bg-white dark:bg-dark-800 shadow-2xl overflow-y-auto">
          <div class="sticky top-0 bg-white dark:bg-dark-800 border-b border-slate-200 dark:border-dark-700 px-6 py-4 flex items-center justify-between">
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">{{ selectedTenant.name }}</h3>
            <button @click="selectedTenant = null" class="p-2 hover:bg-slate-100 dark:hover:bg-dark-700 rounded-lg transition-colors">
              <X class="w-5 h-5 text-slate-500" />
            </button>
          </div>
          <div class="p-6 space-y-6">
            <div class="grid grid-cols-2 gap-4">
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">项目数量</p>
                <p class="text-2xl font-bold text-slate-900 dark:text-white">{{ selectedTenant.projects.length }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">Agent数量</p>
                <p class="text-2xl font-bold text-slate-900 dark:text-white">{{ selectedTenant.agents }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">存储占用</p>
                <p class="text-2xl font-bold text-slate-900 dark:text-white">{{ selectedTenant.storage }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">流量占用</p>
                <p class="text-2xl font-bold text-slate-900 dark:text-white">{{ selectedTenant.traffic }}</p>
              </div>
            </div>

            <div>
              <h4 class="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-3">项目列表</h4>
              <div class="space-y-2">
                <div
                  v-for="project in selectedTenant.projects"
                  :key="project.id"
                  class="p-3 bg-slate-50 dark:bg-dark-700 rounded-lg"
                >
                  <div class="flex items-center justify-between">
                    <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ project.name }}</span>
                    <span class="text-xs text-slate-500">{{ project.agents }} Agents</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { Plus, Building2, X } from 'lucide-vue-next'

const tenants = ref([
  { id: 1, name: '阿里巴巴', projects: [{ id: 1, name: '电商平台', agents: 12 }, { id: 2, name: '物流系统', agents: 8 }], agents: 20, storage: '100 GB', traffic: '5 TB/month' },
  { id: 2, name: '腾讯', projects: [{ id: 3, name: '社交平台', agents: 15 }], agents: 15, storage: '80 GB', traffic: '8 TB/month' },
  { id: 3, name: '字节跳动', projects: [{ id: 4, name: '短视频', agents: 20 }, { id: 5, name: '信息流', agents: 10 }, { id: 6, name: '办公套件', agents: 5 }], agents: 35, storage: '150 GB', traffic: '15 TB/month' },
])

const selectedTenant = ref(null)

const selectTenant = (tenant) => {
  selectedTenant.value = tenant
}
</script>

<style scoped>
.drawer-enter-active,
.drawer-leave-active {
  transition: all 0.3s ease;
}
.drawer-enter-from,
.drawer-leave-to {
  opacity: 0;
}
.drawer-enter-from > div:last-child,
.drawer-leave-to > div:last-child {
  transform: translateX(100%);
}
</style>

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
      <div v-if="loading" class="p-8 text-center text-slate-500">加载中...</div>
      <div v-else-if="tenants.length === 0" class="p-8 text-center text-slate-500">暂无租户数据</div>
      <div v-else class="grid grid-cols-3 gap-4">
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
              <p class="text-xs text-slate-500">{{ tenant.projects?.length || 0 }} 个项目</p>
            </div>
          </div>
          <div class="flex items-center gap-4 text-xs text-slate-500">
            <span>{{ tenant.agents || 0 }} Agents</span>
            <span>{{ tenant.storage || '0 B' }}</span>
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
                <p class="text-2xl font-bold text-slate-900 dark:text-white">{{ selectedTenant.projects?.length || 0 }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">Agent数量</p>
                <p class="text-2xl font-bold text-slate-900 dark:text-white">{{ selectedTenant.agents || 0 }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">存储占用</p>
                <p class="text-2xl font-bold text-slate-900 dark:text-white">{{ selectedTenant.storage || '0 B' }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">流量占用</p>
                <p class="text-2xl font-bold text-slate-900 dark:text-white">{{ selectedTenant.traffic || '0 B' }}</p>
              </div>
            </div>
            <div>
              <h4 class="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-3">项目列表</h4>
              <div v-if="selectedTenant.projects?.length === 0" class="text-center text-slate-500 py-4">暂无项目</div>
              <div v-else class="space-y-2">
                <div
                  v-for="project in selectedTenant.projects"
                  :key="project.id"
                  class="p-3 bg-slate-50 dark:bg-dark-700 rounded-lg"
                >
                  <div class="flex items-center justify-between">
                    <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ project.name }}</span>
                    <span class="text-xs text-slate-500">{{ project.agents || 0 }} Agents</span>
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
import { ref, onMounted } from 'vue'
import { Plus, Building2, X } from 'lucide-vue-next'
import { tenantService } from '@/api'

const tenants = ref([])
const loading = ref(false)
const selectedTenant = ref(null)

const fetchTenants = async () => {
  loading.value = true
  try {
    const res = await tenantService.getTenants()
    tenants.value = res.data || []
  } catch (e) {
    console.error('Failed to fetch tenants:', e)
    tenants.value = []
  } finally {
    loading.value = false
  }
}

const selectTenant = (tenant) => {
  selectedTenant.value = tenant
}

onMounted(fetchTenants)
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

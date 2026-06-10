<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">租户管理</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">管理租户和项目</p>
      </div>
    </div>

    <div class="card">
      <div v-if="loading" class="flex items-center justify-center py-12">
        <Loader2 class="w-8 h-8 text-primary-500 animate-spin" />
      </div>

      <div v-else-if="tenants.length === 0" class="flex flex-col items-center justify-center py-12 text-slate-500">
        <Inbox class="w-12 h-12 mb-3 text-slate-300" />
        <p>暂无数据</p>
      </div>

      <div v-else class="p-6">
        <div class="grid grid-cols-3 gap-4">
          <div
            v-for="(tenant, idx) in tenants"
            :key="idx"
            class="p-4 rounded-xl cursor-pointer transition-all bg-slate-50 dark:bg-dark-700 border-2 border-transparent hover:border-slate-200 dark:hover:border-dark-600"
          >
            <div class="flex items-center gap-3 mb-3">
              <div class="w-10 h-10 rounded-xl bg-primary-100 dark:bg-primary-500/20 flex items-center justify-center">
                <Building2 class="w-5 h-5 text-primary-500" />
              </div>
              <div>
                <h3 class="font-semibold text-slate-900 dark:text-white">
                  {{ tenant.name || tenant.tenant_name || tenant.tenantName || tenant.id || '租户' }}
                </h3>
                <p class="text-xs text-slate-500">
                  {{ (tenant.projects && tenant.projects.length) || tenant.project_count || tenant.projects_count || 0 }} 个项目
                </p>
              </div>
            </div>
            <div class="flex items-center gap-4 text-xs text-slate-500">
              <span>{{ tenant.agent_count || tenant.agents_count || tenant.agents || 0 }} Agents</span>
              <span>{{ tenant.created_at || tenant.createTime || tenant.createdAt || '-' }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { Building2, Loader2, Inbox } from 'lucide-vue-next'
import { tenantService } from '../../../api'

const loading = ref(false)
const tenants = ref([])

const fetchData = async () => {
  loading.value = true
  try {
    const data = await tenantService.getTenants()
    const list = Array.isArray(data) ? data : (data.data || data.items || data.results || data.tenants || [])
    tenants.value = list
  } catch (err) {
    tenants.value = []
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>

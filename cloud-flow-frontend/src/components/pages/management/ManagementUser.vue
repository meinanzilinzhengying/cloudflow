<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">用户管理</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">管理系统用户和权限</p>
      </div>
    </div>

    <div class="card">
      <div class="overflow-x-auto">
        <div v-if="loading" class="flex items-center justify-center py-12">
          <Loader2 class="w-8 h-8 text-primary-500 animate-spin" />
        </div>

        <div v-else-if="users.length === 0" class="flex flex-col items-center justify-center py-12 text-slate-500">
          <Inbox class="w-12 h-12 mb-3 text-slate-300" />
          <p>暂无数据</p>
        </div>

        <table v-else class="w-full">
          <thead>
            <tr class="bg-slate-50 dark:bg-dark-700/50">
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">用户名</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">邮箱</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">角色</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">创建时间</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr v-for="(user, idx) in users" :key="idx">
              <td class="px-6 py-3">
                <div class="flex items-center gap-3">
                  <div class="w-8 h-8 rounded-full bg-slate-200 dark:bg-dark-600 flex items-center justify-center">
                    <User class="w-4 h-4 text-slate-500" />
                  </div>
                  <span class="text-sm font-medium text-slate-900 dark:text-white">
                    {{ user.username || user.name || user.user_name || user.nickname || '-' }}
                  </span>
                </div>
              </td>
              <td class="px-6 py-3 text-sm text-slate-500">{{ user.email || user.mail || '-' }}</td>
              <td class="px-6 py-3">
                <span class="text-xs px-2 py-1 rounded-full bg-primary-50 text-primary-700 dark:bg-primary-500/10 dark:text-primary-400">
                  {{ user.role || user.roles || user.user_role || '-' }}
                </span>
              </td>
              <td class="px-6 py-3 text-sm text-slate-500">
                {{ user.created_at || user.createTime || user.createdAt || user.registration_time || '-' }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { User, Loader2, Inbox } from 'lucide-vue-next'
import { authService } from '../../../api'

const loading = ref(false)
const users = ref([])

const fetchData = async () => {
  loading.value = true
  try {
    const data = await authService.getUsers()
    const list = Array.isArray(data) ? data : (data.data || data.items || data.results || data.users || [])
    users.value = list
  } catch (err) {
    users.value = []
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>

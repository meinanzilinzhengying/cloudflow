<template>
  <div class="space-y-6">
    <!-- Page Header -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">用户管理</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">管理系统用户和权限</p>
      </div>
      <button class="btn-primary">
        <Plus class="w-4 h-4" />
        添加用户
      </button>
    </div>

    <!-- Three Column Layout -->
    <div class="grid grid-cols-3 gap-6">
      <!-- Users -->
      <div class="card">
        <div class="p-4 border-b border-slate-200 dark:border-dark-700">
          <h3 class="text-sm font-semibold text-slate-700 dark:text-slate-200">用户列表</h3>
        </div>
        <div class="p-2">
          <div v-if="loading" class="p-4 text-center text-slate-500">加载中...</div>
          <div v-else-if="users.length === 0" class="p-4 text-center text-slate-500">暂无用户数据</div>
          <div
            v-for="user in users"
            :key="user.id"
            @click="selectUser(user)"
            :class="[
              'p-3 rounded-lg cursor-pointer transition-colors',
              selectedUser?.id === user.id ? 'bg-primary-50 dark:bg-primary-500/10' : 'hover:bg-slate-50 dark:hover:bg-dark-700'
            ]"
          >
            <div class="flex items-center gap-3">
              <div class="w-8 h-8 rounded-full bg-slate-200 dark:bg-dark-600 flex items-center justify-center">
                <User class="w-4 h-4 text-slate-500" />
              </div>
              <div>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ user.name }}</p>
                <p class="text-xs text-slate-500">{{ user.email }}</p>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Roles -->
      <div class="card">
        <div class="p-4 border-b border-slate-200 dark:border-dark-700">
          <h3 class="text-sm font-semibold text-slate-700 dark:text-slate-200">角色列表</h3>
        </div>
        <div class="p-2">
          <div v-if="roles.length === 0" class="p-4 text-center text-slate-500">暂无角色数据</div>
          <div
            v-for="role in roles"
            :key="role.id"
            @click="selectRole(role)"
            :class="[
              'p-3 rounded-lg cursor-pointer transition-colors',
              selectedRole?.id === role.id ? 'bg-accent-50 dark:bg-accent-500/10' : 'hover:bg-slate-50 dark:hover:bg-dark-700'
            ]"
          >
            <div class="flex items-center justify-between">
              <span class="text-sm font-medium text-slate-900 dark:text-white">{{ role.name }}</span>
              <span class="text-xs text-slate-500">{{ role.permissions?.length || 0 }} 权限</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Permissions -->
      <div class="card">
        <div class="p-4 border-b border-slate-200 dark:border-dark-700">
          <h3 class="text-sm font-semibold text-slate-700 dark:text-slate-200">权限树</h3>
        </div>
        <div class="p-4 space-y-2">
          <div v-if="permissions.length === 0" class="text-center text-slate-500">暂无权限数据</div>
          <div v-for="permission in permissions" :key="permission.id" class="space-y-1">
            <div class="flex items-center gap-2">
              <input type="checkbox" :checked="permission.checked" class="rounded" />
              <span class="text-sm text-slate-700 dark:text-slate-200">{{ permission.name }}</span>
            </div>
            <div v-if="permission.children" class="pl-6 space-y-1">
              <div
                v-for="child in permission.children"
                :key="child.id"
                class="flex items-center gap-2"
              >
                <input type="checkbox" :checked="child.checked" class="rounded" />
                <span class="text-xs text-slate-500">{{ child.name }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { Plus, User } from 'lucide-vue-next'
import { authService } from '@/api'

const users = ref([])
const roles = ref([])
const permissions = ref([])
const loading = ref(false)
const selectedUser = ref(null)
const selectedRole = ref(null)

const fetchUsers = async () => {
  loading.value = true
  try {
    const res = await authService.getUsers()
    users.value = res.data || []
  } catch (e) {
    console.error('Failed to fetch users:', e)
    users.value = []
  } finally {
    loading.value = false
  }
}

const fetchRoles = async () => {
  try {
    const res = await authService.getRoles()
    roles.value = res.data || []
  } catch (e) {
    console.error('Failed to fetch roles:', e)
    roles.value = []
  }
}

const fetchPermissions = async () => {
  try {
    const res = await authService.getPermissions()
    permissions.value = res.data || []
  } catch (e) {
    console.error('Failed to fetch permissions:', e)
    permissions.value = []
  }
}

const selectUser = (user) => {
  selectedUser.value = user
}

const selectRole = (role) => {
  selectedRole.value = role
}

onMounted(() => {
  fetchUsers()
  fetchRoles()
  fetchPermissions()
})
</script>

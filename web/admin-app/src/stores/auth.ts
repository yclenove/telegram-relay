import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import * as adminApi from '@/api/admin'
import { TOKEN_KEY } from '@/api/http'

/**
 * 管理端登录态：与 localStorage 同步，供路由守卫与 axios 拦截器侧使用同一 token。
 */
export const useAuthStore = defineStore('auth', () => {
  const token = ref<string>(localStorage.getItem(TOKEN_KEY) ?? '')
  const permissions = ref<string[]>([])

  const isLoggedIn = computed(() => token.value.length > 0)

  function setSession(accessToken: string, perms: string[]) {
    token.value = accessToken
    permissions.value = perms
    localStorage.setItem(TOKEN_KEY, accessToken)
  }

  function clearSession() {
    token.value = ''
    permissions.value = []
    localStorage.removeItem(TOKEN_KEY)
  }

  async function login(username: string, password: string) {
    const res = await adminApi.login(username, password)
    setSession(res.access_token, res.permissions ?? [])
  }

  function logout() {
    clearSession()
  }

  function hasPermission(code: string) {
    return permissions.value.includes(code) || permissions.value.includes('system.manage')
  }

  return {
    token,
    permissions,
    isLoggedIn,
    login,
    logout,
    hasPermission,
    setSession,
    clearSession,
  }
})

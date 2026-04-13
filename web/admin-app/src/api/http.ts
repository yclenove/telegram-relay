import axios, { type AxiosInstance } from 'axios'

const TOKEN_KEY = 'telegram_relay_admin_token'

/**
 * 创建 Axios 实例并挂请求拦截器。
 * 使用 localStorage 读 token，避免 Pinia 与 axios 循环依赖。
 */
function createHttp(): AxiosInstance {
  const instance = axios.create({
    baseURL: import.meta.env.VITE_API_BASE_URL || '',
    timeout: 30000,
    headers: { 'Content-Type': 'application/json' },
  })
  instance.interceptors.request.use((config) => {
    const token = localStorage.getItem(TOKEN_KEY)
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  })
  // 除登录接口外，若 token 失效则清本地状态并回到登录页，避免页面卡在无限 401。
  instance.interceptors.response.use(
    (res) => res,
    (err: unknown) => {
      const ax = err as { response?: { status?: number }; config?: { url?: string } }
      const status = ax.response?.status
      const url = ax.config?.url ?? ''
      if (status === 401 && !url.includes('/auth/login')) {
        localStorage.removeItem(TOKEN_KEY)
        if (typeof window !== 'undefined' && !window.location.pathname.endsWith('/login')) {
          window.location.assign(`${window.location.origin}${import.meta.env.BASE_URL}login`)
        }
      }
      return Promise.reject(err)
    },
  )
  return instance
}

export const http = createHttp()
export { TOKEN_KEY }

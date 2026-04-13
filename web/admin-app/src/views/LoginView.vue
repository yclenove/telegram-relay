<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()

const username = ref('admin')
const password = ref('')
const loading = ref(false)

async function onSubmit() {
  loading.value = true
  try {
    await auth.login(username.value, password.value)
    ElMessage.success('登录成功')
    const redirect = (route.query.redirect as string) || '/dashboard'
    router.replace(redirect)
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e)
    ElMessage.error(`登录失败：${msg}`)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-wrap">
    <el-card class="card" shadow="hover">
      <template #header>
        <span>管理台登录</span>
      </template>
      <el-form label-position="top" @submit.prevent="onSubmit">
        <el-form-item label="用户名">
          <el-input v-model="username" autocomplete="username" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="password" type="password" show-password autocomplete="current-password" />
        </el-form-item>
        <el-button type="primary" :loading="loading" native-type="submit" style="width: 100%">
          登录
        </el-button>
      </el-form>
      <p class="hint">开发时可在项目根配置 <code>.env</code> 的 <code>VITE_API_BASE_URL</code> 指向后端。</p>
    </el-card>
  </div>
</template>

<style scoped>
.login-wrap {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(160deg, #0f172a, #1e1b4b);
}
.card {
  width: 400px;
  max-width: 92vw;
}
.hint {
  margin-top: 12px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>

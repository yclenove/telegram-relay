<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

/** 与路由 path 对齐，供 el-menu default-active 使用 */
const activePath = computed(() => route.path)

function onLogout() {
  auth.logout()
  router.push({ name: 'login' })
}
</script>

<template>
  <el-container class="app-shell">
    <el-aside width="220px" class="aside">
      <div class="brand">Telegram 网关</div>
      <el-menu :default-active="activePath" router>
        <el-menu-item index="/dashboard" :route="{ name: 'dashboard' }">仪表盘</el-menu-item>
        <el-menu-item index="/bots" :route="{ name: 'bots' }">机器人</el-menu-item>
        <el-menu-item index="/rules" :route="{ name: 'rules' }">路由规则</el-menu-item>
        <el-menu-item index="/events" :route="{ name: 'events' }">事件中心</el-menu-item>
        <el-menu-item index="/audits" :route="{ name: 'audits' }">审计日志</el-menu-item>
      </el-menu>
    </el-aside>
    <el-container direction="vertical">
      <el-header class="header">
        <span class="title">{{ (route.meta.title as string) || '' }}</span>
        <div class="right">
          <el-tag v-if="auth.permissions.length" type="info" size="small">
            {{ auth.permissions.join(', ') }}
          </el-tag>
          <el-button type="primary" link @click="onLogout">退出</el-button>
        </div>
      </el-header>
      <el-main class="main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<style scoped>
.app-shell {
  min-height: 100vh;
}
.aside {
  border-right: 1px solid var(--el-border-color);
}
.brand {
  padding: 16px;
  font-weight: 600;
  font-size: 16px;
}
.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--el-border-color);
}
.title {
  font-size: 16px;
}
.right {
  display: flex;
  align-items: center;
  gap: 12px;
}
.main {
  background: var(--el-bg-color-page);
}
</style>

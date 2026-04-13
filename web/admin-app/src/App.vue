<script setup lang="ts">
import { computed, ref } from 'vue'

type Bot = { id: number; name: string; is_default: boolean; is_enabled: boolean; remark: string }
type Rule = { id: number; name: string; priority: number; match_source: string; match_level: string; destination_id: number }
type EventItem = { id: number; event_id: string; source: string; level: string; status: string; created_at: string }

// 管理台页面标签，保持与后端 API 能力一一对应。
const tabs = ['dashboard', 'bots', 'rules', 'events'] as const
type TabKey = (typeof tabs)[number]

const activeTab = ref<TabKey>('dashboard')
const username = ref('admin')
const password = ref('')
const loginMessage = ref('')
const token = ref('')
const loading = ref(false)
const stats = ref<Record<string, number>>({})
const bots = ref<Bot[]>([])
const rules = ref<Rule[]>([])
const events = ref<EventItem[]>([])

const botForm = ref({ name: '', botToken: '', remark: '', isDefault: false })
const ruleForm = ref({ name: '', priority: 100, source: '', level: '', destinationID: 0 })

const apiBase = computed(() => import.meta.env.VITE_API_BASE_URL || '')

async function request(path: string, init: RequestInit = {}) {
  const headers = new Headers(init.headers || {})
  headers.set('Content-Type', 'application/json')
  if (token.value) headers.set('Authorization', `Bearer ${token.value}`)
  const response = await fetch(`${apiBase.value}${path}`, { ...init, headers })
  if (!response.ok) throw new Error(await response.text())
  return response.json()
}

async function login() {
  loading.value = true
  loginMessage.value = ''
  try {
    const data = await request('/api/v2/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username: username.value, password: password.value }),
    })
    token.value = data.access_token
    loginMessage.value = `登录成功，权限：${(data.permissions || []).join(', ')}`
    await loadAll()
  } catch (error) {
    loginMessage.value = `登录失败：${String(error)}`
  } finally {
    loading.value = false
  }
}

async function loadAll() {
  await Promise.all([loadDashboard(), loadBots(), loadRules(), loadEvents()])
}

async function loadDashboard() {
  stats.value = await request('/api/v2/dashboard')
}

async function loadBots() {
  bots.value = await request('/api/v2/bots')
}

async function createBot() {
  await request('/api/v2/bots', {
    method: 'POST',
    body: JSON.stringify({
      name: botForm.value.name,
      bot_token: botForm.value.botToken,
      is_default: botForm.value.isDefault,
      remark: botForm.value.remark,
    }),
  })
  botForm.value = { name: '', botToken: '', remark: '', isDefault: false }
  await loadBots()
}

async function loadRules() {
  rules.value = await request('/api/v2/rules')
}

async function createRule() {
  await request('/api/v2/rules', {
    method: 'POST',
    body: JSON.stringify({
      name: ruleForm.value.name,
      priority: Number(ruleForm.value.priority),
      match_source: ruleForm.value.source,
      match_level: ruleForm.value.level,
      destination_id: Number(ruleForm.value.destinationID),
    }),
  })
  ruleForm.value = { name: '', priority: 100, source: '', level: '', destinationID: 0 }
  await loadRules()
}

async function loadEvents() {
  events.value = await request('/api/v2/events')
}
</script>

<template>
  <div class="layout">
    <aside class="sidebar">
      <h1>网关管理台</h1>
      <button v-for="tab in tabs" :key="tab" :class="{ active: activeTab === tab }" @click="activeTab = tab">
        {{ tab }}
      </button>
    </aside>
    <main class="main">
      <section class="card">
        <h2>登录</h2>
        <div class="grid two">
          <label>用户名<input v-model="username" /></label>
          <label>密码<input v-model="password" type="password" /></label>
        </div>
        <button :disabled="loading" @click="login">{{ loading ? '登录中...' : '登录并加载数据' }}</button>
        <p class="tip">{{ loginMessage }}</p>
      </section>

      <section v-if="activeTab === 'dashboard'" class="card">
        <h2>仪表盘</h2>
        <div class="grid stats">
          <div v-for="(value, key) in stats" :key="key" class="stat">
            <small>{{ key }}</small>
            <strong>{{ value }}</strong>
          </div>
        </div>
      </section>

      <section v-if="activeTab === 'bots'" class="card">
        <h2>机器人管理</h2>
        <div class="grid two">
          <label>名称<input v-model="botForm.name" /></label>
          <label>Bot Token<input v-model="botForm.botToken" /></label>
          <label>备注<input v-model="botForm.remark" /></label>
          <label class="checkbox"><input v-model="botForm.isDefault" type="checkbox" />设为默认机器人</label>
        </div>
        <button @click="createBot">创建机器人</button>
        <table>
          <thead><tr><th>ID</th><th>名称</th><th>默认</th><th>启用</th><th>备注</th></tr></thead>
          <tbody>
            <tr v-for="item in bots" :key="item.id">
              <td>{{ item.id }}</td><td>{{ item.name }}</td><td>{{ item.is_default }}</td><td>{{ item.is_enabled }}</td><td>{{ item.remark }}</td>
            </tr>
          </tbody>
        </table>
      </section>

      <section v-if="activeTab === 'rules'" class="card">
        <h2>路由规则</h2>
        <div class="grid two">
          <label>规则名称<input v-model="ruleForm.name" /></label>
          <label>优先级<input v-model.number="ruleForm.priority" type="number" /></label>
          <label>匹配来源<input v-model="ruleForm.source" /></label>
          <label>匹配级别<input v-model="ruleForm.level" /></label>
          <label>目标 Destination ID<input v-model.number="ruleForm.destinationID" type="number" /></label>
        </div>
        <button @click="createRule">创建规则</button>
        <table>
          <thead><tr><th>ID</th><th>名称</th><th>优先级</th><th>来源</th><th>级别</th><th>目标</th></tr></thead>
          <tbody>
            <tr v-for="item in rules" :key="item.id">
              <td>{{ item.id }}</td><td>{{ item.name }}</td><td>{{ item.priority }}</td><td>{{ item.match_source || '-' }}</td><td>{{ item.match_level || '-' }}</td><td>{{ item.destination_id }}</td>
            </tr>
          </tbody>
        </table>
      </section>

      <section v-if="activeTab === 'events'" class="card">
        <h2>事件中心</h2>
        <button @click="loadEvents">刷新</button>
        <table>
          <thead><tr><th>ID</th><th>EventID</th><th>来源</th><th>级别</th><th>状态</th><th>时间</th></tr></thead>
          <tbody>
            <tr v-for="item in events" :key="item.id">
              <td>{{ item.id }}</td><td>{{ item.event_id }}</td><td>{{ item.source }}</td><td>{{ item.level }}</td><td>{{ item.status }}</td><td>{{ item.created_at }}</td>
            </tr>
          </tbody>
        </table>
      </section>
    </main>
  </div>
</template>

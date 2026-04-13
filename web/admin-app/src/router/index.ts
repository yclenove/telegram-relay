import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const LoginView = () => import('@/views/LoginView.vue')
const MainLayout = () => import('@/layouts/MainLayout.vue')
const DashboardView = () => import('@/views/DashboardView.vue')
const BotsView = () => import('@/views/BotsView.vue')
const RulesView = () => import('@/views/RulesView.vue')
const EventsView = () => import('@/views/EventsView.vue')
const AuditsView = () => import('@/views/AuditsView.vue')

export const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    { path: '/login', name: 'login', component: LoginView, meta: { public: true } },
    {
      path: '/',
      component: MainLayout,
      meta: { requiresAuth: true },
      children: [
        { path: '', redirect: { name: 'dashboard' } },
        { path: 'dashboard', name: 'dashboard', component: DashboardView, meta: { title: '仪表盘' } },
        { path: 'bots', name: 'bots', component: BotsView, meta: { title: '机器人', perm: 'bot.manage' } },
        { path: 'rules', name: 'rules', component: RulesView, meta: { title: '路由规则', perm: 'rule.manage' } },
        { path: 'events', name: 'events', component: EventsView, meta: { title: '事件', perm: 'event.read' } },
        { path: 'audits', name: 'audits', component: AuditsView, meta: { title: '审计', perm: 'audit.read' } },
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: '/dashboard' },
  ],
})

router.beforeEach((to, _from, next) => {
  const auth = useAuthStore()
  if (to.meta.public) {
    if (auth.isLoggedIn && to.path === '/login') {
      next({ name: 'dashboard' })
      return
    }
    next()
    return
  }
  if (to.meta.requiresAuth && !auth.isLoggedIn) {
    next({ name: 'login', query: { redirect: to.fullPath } })
    return
  }
  const perm = to.meta.perm as string | undefined
  if (perm && !auth.hasPermission(perm)) {
    next({ name: 'dashboard' })
    return
  }
  next()
})

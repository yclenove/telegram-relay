import { createApp } from 'vue'
import { createPinia } from 'pinia'
// 样式全量引入；组件由 unplugin-vue-components 按需注册，避免打包体积过大。
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'

import App from './App.vue'
import { router } from './router'
import './style.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')

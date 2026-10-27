import './assets/main.css'

import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'

import App from './App.vue'
import router from './router'
// 统一请求层：挂上 token 注入与 401 跳登录的拦截器。
// 必须在任何组件发请求之前 import（这里在 mount 之前就生效）。
import './api/client'

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(ElementPlus) // 注册 Element Plus 组件库

app.mount('#app')

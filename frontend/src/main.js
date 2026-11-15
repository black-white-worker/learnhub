import './assets/main.css'

import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import 'element-plus/dist/index.css'

import App from './App.vue'
import router from './router'
// 统一请求层：挂上 token 注入与 401 跳登录的拦截器。
// 必须在任何组件发请求之前 import（这里在 mount 之前就生效）。
import './api/client'

const app = createApp(App)

app.use(createPinia())
app.use(router)
// 中文语言包：不配的话空下拉框会显示英文 "Select"、分页显示 "Total" 等，
// 中文出题页尤其明显。统一注册后所有组件占位/分页/日期文案都是简体中文。
app.use(ElementPlus, { locale: zhCn })

app.mount('#app')

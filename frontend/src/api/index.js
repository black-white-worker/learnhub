import axios from 'axios'

// 统一的后端请求实例：
// baseURL 为 /api，配合 vite.config.js 里的代理，
// 浏览器请求 /api/xxx 会由 Vite 转发到后端 http://localhost:8888/api/xxx
const request = axios.create({
  baseURL: '/api',
  timeout: 10000,
})

export default request

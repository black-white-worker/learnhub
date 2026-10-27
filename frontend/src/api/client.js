// 统一的前端请求层。
//
// 改造前的问题：
//   1. 大约 60 处重复写 `headers: { Authorization: 'Bearer ' + token }`，
//      漏写一处就是"神秘 401"；
//   2. 没有任何统一处理 401 的地方 —— token 24 小时过期后，
//      用户看到的是一堆"加载失败"，而不是"请重新登录"；
//   3. 后端错误是 {code,message}，每个页面自己 `e.response?.data?.message`，
//      还出现过读 `.msg` 取不到提示的 bug。
//
// 这里的做法：
//   - 给 **全局 axios** 挂请求/响应拦截器（现有视图里手写的 axios.get 也自动受益）；
//   - 额外导出一个 client 实例，自动解包 {code,message,data}，新代码统一用它；
//   - 401 统一清会话 + 跳登录，并在跳转前记住原始地址。

import axios from 'axios'
import { ElMessage } from 'element-plus'

// ===== 1. 让所有 axios 请求自动带 token =====
axios.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token && !config.headers?.Authorization) {
    config.headers = config.headers || {}
    config.headers.Authorization = 'Bearer ' + token
  }
  return config
})

// ===== 2. 401 统一处理（清会话 + 跳登录）=====
let redirecting = false

function handleUnauthorized() {
  if (redirecting) return
  redirecting = true
  localStorage.removeItem('token')
  localStorage.removeItem('username')
  localStorage.removeItem('role')
  const current = window.location.pathname + window.location.search
  // 已经在登录页就不重复跳
  if (!current.startsWith('/login')) {
    window.location.href = '/login?redirect=' + encodeURIComponent(current)
  }
  setTimeout(() => { redirecting = false }, 2000)
}

// 统一的错误消息提取：兼容 {message} / {msg} / axios 原生 message
export function errorMessage(err, fallback = '操作失败') {
  const data = err?.response?.data
  if (typeof data === 'string' && data.trim()) return data
  return data?.message || data?.msg || err?.message || fallback
}

function attachResponseInterceptor(instance, { unwrap }) {
  instance.interceptors.response.use(
    (res) => {
      if (res.status === 401) {
        handleUnauthorized()
        return Promise.reject(new Error('未登录'))
      }
      if (unwrap) {
        const body = res.data
        // 后端新接口是 {code,message,data}；老接口是裸 data，两种都兼容
        if (body && typeof body === 'object' && !Array.isArray(body) && 'code' in body) {
          if (body.code === 0) return body.data
          return Promise.reject(new Error(body.message || '请求失败'))
        }
        return body
      }
      return res
    },
    (err) => {
      if (err?.response?.status === 401) {
        handleUnauthorized()
      }
      // 把后端的中文提示挂到 error.message 上，调用方直接 toast 即可
      err.message = errorMessage(err)
      return Promise.reject(err)
    }
  )
}

attachResponseInterceptor(axios, { unwrap: false })

// ===== 3. 新代码用的 client：baseURL=/api + 自动解包 =====
const client = axios.create({
  baseURL: '/api',
  timeout: 20000,
})

attachResponseInterceptor(client, { unwrap: true })

// ===== 4. 老代码用的 rawClient：同一套拦截器，但保留 axios 原始响应 =====
//
// 历史页面都按 `res.data.xxx` 取值，一次性改成解包版会牵动 19 个视图。
// 这里提供一个"不自动解包"的实例，让所有请求仍然只经过一份拦截器实现
// （token 注入、401 跳转、错误消息提取），新代码继续用上面的 client。
export const rawClient = axios.create({
  baseURL: '/api',
  timeout: 20000,
})

attachResponseInterceptor(rawClient, { unwrap: false })

// 下载文件（成绩单 CSV 等）：必须走 blob，不能走自动解包
export async function downloadFile(path, fallbackName = 'download') {
  const res = await axios.get(path, { responseType: 'blob', timeout: 60000 })
  const blob = new Blob([res.data])
  const disposition = res.headers?.['content-disposition'] || ''
  let filename = fallbackName
  const star = /filename\*=UTF-8''([^;]+)/i.exec(disposition)
  const plain = /filename="?([^";]+)"?/i.exec(disposition)
  if (star) filename = decodeURIComponent(star[1])
  else if (plain) filename = plain[1]

  const url = window.URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  window.URL.revokeObjectURL(url)
}

// 统一 toast 错误（页面里不用再写 ElMessage.error(e.response?.data?.message)）
export function toastError(err, fallback) {
  ElMessage.error(errorMessage(err, fallback))
}

export default client

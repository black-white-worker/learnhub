import axios from 'axios'

// 统一的后端请求实例：
// baseURL 为 /api，配合 vite.config.js 里的代理，
// 浏览器请求 /api/xxx 会由 Vite 转发到后端 http://localhost:8888/api/xxx
const request = axios.create({
  baseURL: '/api',
  timeout: 10000,
})

// 登录接口：提交 {username, password}，成功返回 {token, username}
export function login(data) {
  return request.post('/user/login', data)
}

// 注册接口：提交 {username, password}，成功返回 {username}
export function register(data) {
  return request.post('/user/register', data)
}

// 课程创建接口：提交 {name, intro, teacher}，成功返回 {id, name}
export function createCourse(data) {
  return request.post('/course/create', data)
}

export default request

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

// 课程创建接口（来自「课程创建」分支）：提交 {name, intro, teacher}，成功返回 {id, name}
export function createCourse(data) {
  return request.post('/course/create', data)
}

// 课程列表接口（来自「课程列表」分支）：GET /api/course/list，成功返回 {courses: [...]}
export function fetchCourseList() {
  return request.get('/course/list')
}

// 课程封面上传接口：提交 FormData（courseId + file）
// 成功返回 {courseId, coverUrl, coverThumbUrl}
// 注意：data 是 FormData 时 axios 会自动带上 multipart/form-data 及 boundary，
// 不要手动设置 Content-Type，否则会丢失 boundary 导致后端解析失败
export function uploadCourseCover(data) {
  return request.post('/course/upload/cover', data)
}

// 课程视频上传接口：提交 FormData（courseId + files，支持一次多个视频文件）
// 成功返回 {courseId, videos: [{id, name, url, size, uploadedAt}]}
export function uploadCourseVideos(data) {
  return request.post('/course/upload/videos', data)
}

// ===== 以下为「课程观看」分支新增 =====

// 获取视频播放信息：GET /api/course/video/play/:courseId/:videoId
export function getVideoPlay(courseId, videoId) {
  return request.get(`/course/video/play/${courseId}/${videoId}`)
}

// 上报播放进度（position 单位：秒）
export function saveVideoProgress(courseId, videoId, position) {
  return request.post(`/course/video/progress/${courseId}/${videoId}`, { position })
}

// 读取上次播放进度（秒）
export function fetchVideoProgress(courseId, videoId) {
  return request.get(`/course/video/progress/${courseId}/${videoId}`)
}

export default request

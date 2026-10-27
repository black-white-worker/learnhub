import axios from 'axios'

// 统一的后端请求实例：
// baseURL 为 /api，配合 vite.config.js 里的代理
const request = axios.create({
  baseURL: '/api',
  timeout: 10000,
})

// 全局请求拦截器：自动从 localStorage 读取 token 并带上 Authorization 头。
// 课程路由已加登录中间件，不带 token 会返回 401，导致首页课程列表为空。
request.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = 'Bearer ' + token
  }
  return config
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

// 上报播放进度。
// 改造后按用户落库并统计"真实观看时长"，所以除了位置还要上报：
//   watchedDelta  上次上报以来真实播放的秒数（拖动不计）
//   duration      视频总时长（服务端据此判断是否看完）
// 兼容：老调用方式 saveVideoProgress(cid, vid, 42) 仍然可用。
export function saveVideoProgress(courseId, videoId, positionOrOpts) {
  const payload =
    typeof positionOrOpts === 'number'
      ? { position: positionOrOpts }
      : { position: 0, watchedDelta: 0, duration: 0, ...positionOrOpts }
  return request.post(`/course/video/progress/${courseId}/${videoId}`, payload)
}

// 读取上次播放进度（秒）
export function fetchVideoProgress(courseId, videoId) {
  return request.get(`/course/video/progress/${courseId}/${videoId}`)
}

// ===== 以下为「课程资源」分支新增 =====

// 上传课程文档资源（PPT / Word / PDF）：提交 FormData（courseId + file）
// 成功返回 {courseId, resource: {id, name, url, size, type, uploadedAt}}
export function uploadCourseResource(data) {
  return request.post('/course/upload/resource', data)
}

// 课程资源列表：GET /api/course/resources/:courseId
export function fetchCourseResources(courseId) {
  return request.get(`/course/resources/${courseId}`)
}

// 删除课程资源：DELETE /api/course/resource/:courseId/:resourceId
export function deleteCourseResource(courseId, resourceId) {
  return request.delete(`/course/resource/${courseId}/${resourceId}`)
}

// ===== 以下为「课程编辑」分支新增 =====

// 课程详情接口：GET /api/course/detail/:id
// 成功返回课程完整信息（封面 / 视频列表），供课程详情页使用
export function getCourseDetail(id) {
  return request.get(`/course/detail/${id}`)
}

// 编辑课程基本信息：PUT /api/course/edit，提交 {id, name, intro, teacher}
export function editCourse(data) {
  return request.put('/course/edit', data)
}

// 删除课程：DELETE /api/course/delete/:id
export function deleteCourse(id) {
  return request.delete(`/course/delete/${id}`)
}

export default request

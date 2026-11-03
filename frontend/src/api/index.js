// 前端 API 层。
//
// 约定：
//   - 所有请求都走 client.js 里的实例（统一注入 token、统一 401 跳转）；
//   - 老页面按 `res.data.xxx` 取值，所以这里用 rawClient（不解包信封）；
//     新代码可以直接用 client.js 默认导出的 client（自动解包 + 抛错）；
//   - 上传类接口的 courseId 放在 query 里：后端需要"先鉴权、再读 body"，
//     避免未授权请求先把大文件写进临时目录。

import { rawClient as request } from './client.js'

// ===== 认证 =====
export function login(data) {
  return request.post('/user/login', data)
}

export function register(data) {
  return request.post('/user/register', data)
}

// 退出登录：必须调后端删除 Redis 会话，否则 token 在 24 小时内仍然有效
export function logout() {
  return request.post('/user/logout')
}

// 当前用户信息（角色 / 是否需要改密以服务端为准）
export function fetchInfo() {
  return request.get('/user/info')
}

// 修改自己的密码（必须提供旧密码）
export function changePassword(data) {
  return request.post('/user/password', data)
}

// ===== 课程 =====
export function createCourse(data) {
  return request.post('/course/create', data)
}

export function fetchCourseList() {
  return request.get('/course/list')
}

export function getCourseDetail(id) {
  return request.get(`/course/detail/${id}`)
}

export function editCourse(data) {
  return request.put('/course/edit', data)
}

export function deleteCourse(id) {
  return request.delete(`/course/delete/${id}`)
}

// 上传：courseId 走 query，FormData 只放文件。
// 超时单独放宽到 10 分钟：500MB 视频在慢网络下远超默认 20 秒。
const UPLOAD_TIMEOUT = 10 * 60 * 1000

export function uploadCourseCover(courseId, data) {
  return request.post('/course/upload/cover', data, { params: { courseId }, timeout: UPLOAD_TIMEOUT })
}

export function uploadCourseVideos(courseId, data) {
  return request.post('/course/upload/videos', data, { params: { courseId }, timeout: UPLOAD_TIMEOUT })
}

export function uploadCourseResource(courseId, data) {
  return request.post('/course/upload/resource', data, { params: { courseId }, timeout: UPLOAD_TIMEOUT })
}

// ===== 课程观看 / 资源 =====
export function getVideoPlay(courseId, videoId) {
  return request.get(`/course/video/play/${courseId}/${videoId}`)
}

export function saveVideoProgress(courseId, videoId, positionOrOpts) {
  const payload =
    typeof positionOrOpts === 'number'
      ? { position: positionOrOpts }
      : { position: 0, watchedDelta: 0, duration: 0, ...positionOrOpts }
  return request.post(`/course/video/progress/${courseId}/${videoId}`, payload)
}

export function fetchVideoProgress(courseId, videoId) {
  return request.get(`/course/video/progress/${courseId}/${videoId}`)
}

export function fetchCourseResources(courseId) {
  return request.get(`/course/resources/${courseId}`)
}

export function deleteCourseResource(courseId, resourceId) {
  return request.delete(`/course/resource/${courseId}/${resourceId}`)
}

// ===== 考试 =====
export function getExamDetail(id) {
  return request.get(`/exam/${id}`)
}

export function startExam(id) {
  return request.post(`/exam/${id}/start`)
}

// 保存作答草稿：考试进行中每 15 秒左右调一次，超时收卷按最后一份判分
export function saveExamDraft(id, answers) {
  return request.post(`/exam/${id}/draft`, { answers })
}

export function submitExam(id, answers) {
  return request.post(`/exam/${id}/submit`, { answers })
}

export function gradeExamSubmission(submissionId, data) {
  return request.post(`/exam/submission/${submissionId}/grade`, data)
}

export function reportExamViolation(id) {
  return request.post(`/exam/${id}/violation`)
}

// ===== 消息中心 =====
export function fetchNotifications() {
  return request.get('/notifications')
}

// 通知偏好：列出当前用户的渠道开关（inapp/email）
export function fetchNotificationPrefs() {
  return request.get('/notification-prefs')
}

// 更新某渠道的通知偏好（data: { channel: 'inapp' | 'email', enabled: 1 | 0 }）
export function updateNotificationPrefs(data) {
  return request.post('/notification-prefs', data)
}

export default request

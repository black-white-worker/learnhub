// 会话与考试相关 API 的单元测试。
//
// 重点验证"行为契约"而不是实现：
//   - 退出登录必须打到后端（否则服务端 token 还能用 24 小时）；
//   - 草稿保存走 /exam/:id/draft，载荷是 {answers}；
//   - 交卷/批改/违纪上报的路径不能被改错。

import { test } from 'node:test'
import assert from 'node:assert/strict'

import request, {
  logout,
  fetchInfo,
  changePassword,
  startExam,
  saveExamDraft,
  submitExam,
  gradeExamSubmission,
  reportExamViolation,
} from '../src/api/index.js'

const calls = []
request.interceptors.request.use((config) => {
  calls.push({
    method: String(config.method).toLowerCase(),
    url: config.url,
    data: typeof config.data === 'string' ? JSON.parse(config.data) : config.data,
  })
  throw new Error('intercepted')
})

async function expectReject(p) {
  try {
    await p
  } catch {
    return
  }
}

function lastCall() {
  return calls[calls.length - 1]
}

test('logout() → POST /user/logout（必须让服务端删会话）', async () => {
  await expectReject(logout())
  assert.equal(lastCall().method, 'post')
  assert.equal(lastCall().url, '/user/logout')
})

test('fetchInfo() → GET /user/info', async () => {
  await expectReject(fetchInfo())
  assert.equal(lastCall().method, 'get')
  assert.equal(lastCall().url, '/user/info')
})

test('changePassword() → POST /user/password 且带旧密码', async () => {
  await expectReject(changePassword({ oldPassword: 'old', newPassword: 'new123456' }))
  const call = lastCall()
  assert.equal(call.method, 'post')
  assert.equal(call.url, '/user/password')
  assert.deepEqual(call.data, { oldPassword: 'old', newPassword: 'new123456' })
})

test('startExam(5) → POST /exam/5/start', async () => {
  await expectReject(startExam(5))
  assert.equal(lastCall().method, 'post')
  assert.equal(lastCall().url, '/exam/5/start')
})

test('saveExamDraft(5, answers) → POST /exam/5/draft，载荷为 {answers}', async () => {
  await expectReject(saveExamDraft(5, { 11: 'A', 12: 'true' }))
  const call = lastCall()
  assert.equal(call.method, 'post')
  assert.equal(call.url, '/exam/5/draft')
  assert.deepEqual(call.data, { answers: { 11: 'A', 12: 'true' } })
})

test('submitExam(5, answers) → POST /exam/5/submit', async () => {
  await expectReject(submitExam(5, { 11: 'A' }))
  assert.equal(lastCall().url, '/exam/5/submit')
  assert.deepEqual(lastCall().data, { answers: { 11: 'A' } })
})

test('gradeExamSubmission(9, ...) → POST /exam/submission/9/grade', async () => {
  await expectReject(gradeExamSubmission(9, { teacherScore: 88, comment: '不错' }))
  const call = lastCall()
  assert.equal(call.method, 'post')
  assert.equal(call.url, '/exam/submission/9/grade')
  assert.deepEqual(call.data, { teacherScore: 88, comment: '不错' })
})

test('reportExamViolation(5) → POST /exam/5/violation', async () => {
  await expectReject(reportExamViolation(5))
  assert.equal(lastCall().method, 'post')
  assert.equal(lastCall().url, '/exam/5/violation')
})

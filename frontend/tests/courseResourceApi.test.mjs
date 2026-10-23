// 课程资源分支新增 API 的单元测试（Node 内置测试运行器）
// 用请求拦截器记录 method + url 后中断，不真正发请求。

import { test } from 'node:test'
import assert from 'node:assert/strict'

import request, {
  uploadCourseResource,
  fetchCourseResources,
  deleteCourseResource,
} from '../src/api/index.js'

const calls = []
request.interceptors.request.use((config) => {
  calls.push({ method: String(config.method).toLowerCase(), url: config.url })
  throw new Error('intercepted')
})

async function expectReject(p) {
  try {
    await p
  } catch {
    return
  }
}

test('uploadCourseResource(FormData) → POST /course/upload/resource', async () => {
  await expectReject(uploadCourseResource(new FormData()))
  const last = calls[calls.length - 1]
  assert.equal(last.method, 'post')
  assert.equal(last.url, '/course/upload/resource')
})

test('fetchCourseResources(3) → GET /course/resources/3', async () => {
  await expectReject(fetchCourseResources(3))
  const last = calls[calls.length - 1]
  assert.equal(last.method, 'get')
  assert.equal(last.url, '/course/resources/3')
})

test('deleteCourseResource(3, 5) → DELETE /course/resource/3/5', async () => {
  await expectReject(deleteCourseResource(3, 5))
  const last = calls[calls.length - 1]
  assert.equal(last.method, 'delete')
  assert.equal(last.url, '/course/resource/3/5')
})

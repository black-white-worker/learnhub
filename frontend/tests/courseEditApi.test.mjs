// 课程编辑分支新增 API 的单元测试（Node 内置测试运行器）
// 通过给 axios 实例挂请求拦截器记录 method + url 后立刻中断，
// 不真正发请求，只验证 getCourseDetail / editCourse / deleteCourse 拼对了地址。

import { test } from 'node:test'
import assert from 'node:assert/strict'

import request, { getCourseDetail, editCourse, deleteCourse } from '../src/api/index.js'

const calls = []
request.interceptors.request.use((config) => {
  calls.push({ method: String(config.method).toLowerCase(), url: config.url })
  // 抛出错误中断请求，避免真的发出去
  throw new Error('intercepted')
})

async function expectReject(p) {
  try {
    await p
  } catch {
    return
  }
}

test('getCourseDetail(3) → GET /course/detail/3', async () => {
  await expectReject(getCourseDetail(3))
  const last = calls[calls.length - 1]
  assert.equal(last.method, 'get')
  assert.equal(last.url, '/course/detail/3')
})

test('editCourse({id:3,name:"概率论"}) → PUT /course/edit', async () => {
  await expectReject(editCourse({ id: 3, name: '概率论' }))
  const last = calls[calls.length - 1]
  assert.equal(last.method, 'put')
  assert.equal(last.url, '/course/edit')
})

test('deleteCourse(3) → DELETE /course/delete/3', async () => {
  await expectReject(deleteCourse(3))
  const last = calls[calls.length - 1]
  assert.equal(last.method, 'delete')
  assert.equal(last.url, '/course/delete/3')
})

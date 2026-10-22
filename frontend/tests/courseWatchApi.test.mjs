// 课程观看分支新增 API 的单元测试（Node 内置测试运行器）

import { test } from 'node:test'
import assert from 'node:assert/strict'

import request, {
  getVideoPlay,
  saveVideoProgress,
  fetchVideoProgress,
} from '../src/api/index.js'

const calls = []
request.interceptors.request.use((config) => {
  calls.push({ method: String(config.method).toLowerCase(), url: config.url, data: config.data })
  throw new Error('intercepted')
})

async function expectReject(p) {
  try {
    await p
  } catch {
    return
  }
}

test('getVideoPlay(1,2) → GET /course/video/play/1/2', async () => {
  await expectReject(getVideoPlay(1, 2))
  const last = calls[calls.length - 1]
  assert.equal(last.method, 'get')
  assert.equal(last.url, '/course/video/play/1/2')
})

test('saveVideoProgress(1,2,42) → POST /course/video/progress/1/2', async () => {
  await expectReject(saveVideoProgress(1, 2, 42))
  const last = calls[calls.length - 1]
  assert.equal(last.method, 'post')
  assert.equal(last.url, '/course/video/progress/1/2')
})

test('fetchVideoProgress(1,2) → GET /course/video/progress/1/2', async () => {
  await expectReject(fetchVideoProgress(1, 2))
  const last = calls[calls.length - 1]
  assert.equal(last.method, 'get')
  assert.equal(last.url, '/course/video/progress/1/2')
})

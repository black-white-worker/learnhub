// 视频后处理状态的展示规则。
// 重点盯两个坑：老数据没有 processStatus 字段（只能按 faststarted 推断，
// 不能显示成"转码失败"）；轮询的启停只看还有没有在跑的任务。
import { test } from 'node:test'
import assert from 'node:assert/strict'

import {
  normalizeProcessStatus,
  processStatusTag,
  canRetryProcess,
  hasRunningProcess,
} from '../src/utils/videoStatus.js'

test('normalizeProcessStatus 原样接受后端四种状态', () => {
  for (const s of ['pending', 'processing', 'done', 'failed']) {
    assert.equal(normalizeProcessStatus({ processStatus: s }), s)
  }
})

test('normalizeProcessStatus 大小写与空白容错', () => {
  assert.equal(normalizeProcessStatus({ processStatus: ' DONE ' }), 'done')
})

test('老数据缺 processStatus 时按 faststarted 推断，不会误报失败', () => {
  assert.equal(normalizeProcessStatus({ faststarted: true }), 'done')
  assert.equal(normalizeProcessStatus({ faststarted: false }), 'pending')
  assert.equal(normalizeProcessStatus({}), 'pending')
  assert.equal(normalizeProcessStatus(null), 'pending')
})

test('无法识别的状态值也退回推断，不会显示空白标签', () => {
  assert.equal(normalizeProcessStatus({ processStatus: 'weird', faststarted: true }), 'done')
  const tag = processStatusTag({ processStatus: 'weird' })
  assert.equal(tag.status, 'pending')
  assert.ok(tag.text)
})

test('未完成不给 danger 色，失败才带后端原因', () => {
  assert.equal(processStatusTag({ processStatus: 'pending' }).type, 'info')
  assert.equal(processStatusTag({ processStatus: 'processing' }).type, 'info')
  assert.equal(processStatusTag({ processStatus: 'done' }).type, 'success')
  const failed = processStatusTag({ processStatus: 'failed', processError: '文件格式不支持' })
  assert.equal(failed.type, 'danger')
  assert.equal(failed.hint, '文件格式不支持')
})

test('失败但后端没给原因时用兜底文案', () => {
  const tag = processStatusTag({ processStatus: 'failed' })
  assert.ok(tag.hint.includes('重试'))
})

test('只有失败状态允许人工重试', () => {
  assert.equal(canRetryProcess({ processStatus: 'failed' }), true)
  assert.equal(canRetryProcess({ processStatus: 'pending' }), false)
  assert.equal(canRetryProcess({ processStatus: 'processing' }), false)
  assert.equal(canRetryProcess({ processStatus: 'done' }), false)
  // 老数据推断出来是 pending，不该冒出重试按钮
  assert.equal(canRetryProcess({ faststarted: false }), false)
})

test('hasRunningProcess 决定轮询启停', () => {
  assert.equal(hasRunningProcess([]), false)
  assert.equal(hasRunningProcess(null), false)
  assert.equal(
    hasRunningProcess([{ processStatus: 'done' }, { processStatus: 'processing' }]),
    true,
  )
  assert.equal(hasRunningProcess([{ processStatus: 'done' }, { processStatus: 'failed' }]), false)
  // 失败不轮询：重试之后前端会本地置 pending，才会继续轮询
  assert.equal(hasRunningProcess([{ faststarted: false }]), true)
})

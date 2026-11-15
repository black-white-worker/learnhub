// 上传进度算法的单测：三个上传入口（资源面板 / 课程详情 / 列表快捷上传 / 创建页）
// 共用这套规则，所以这里锁死三件事：字节封顶 99、发完自动切"服务端处理中"、
// 以及文案拼装不会因为缺字段而写出噪音。

import { test } from 'node:test'
import assert from 'node:assert/strict'

import {
  DONE_PERCENT,
  TRANSFER_CEIL,
  applyProgressEvent,
  batchLabel,
  formatBytes,
  makeUploadProgressReporter,
  newProgressState,
  phaseLabel,
  progressLine,
  progressText,
  readProgressEvent,
  resetProgressState,
  transferPercent,
} from '../src/utils/uploadProgress.js'

const MB = 1024 * 1024

test('transferPercent：按比例取整', () => {
  assert.equal(transferPercent(50 * MB, 100 * MB), 50)
  assert.equal(transferPercent(33 * MB, 100 * MB), 33)
})

test('transferPercent：字节发满也只给 99，留 100 给服务端回包', () => {
  assert.equal(transferPercent(100 * MB, 100 * MB), TRANSFER_CEIL)
  // 浏览器偶尔会报超过 total 的字节数（重传同一批），不能溢出成 120%
  assert.equal(transferPercent(120 * MB, 100 * MB), TRANSFER_CEIL)
})

test('transferPercent：总量未知 / 非法时给 0，调用方退回纯文案进度', () => {
  assert.equal(transferPercent(10 * MB, 0), 0)
  assert.equal(transferPercent(10 * MB, undefined), 0)
  assert.equal(transferPercent(0, 100 * MB), 0)
})

test('DONE_PERCENT 就是 100，别让两个常量各自漂移', () => {
  assert.equal(DONE_PERCENT, 100)
})

test('readProgressEvent：没有 total 时 computable=false', () => {
  assert.deepEqual(readProgressEvent({ loaded: 5, total: 20 }), {
    loaded: 5,
    total: 20,
    computable: true,
  })
  assert.equal(readProgressEvent({ loaded: 5 }).computable, false)
  assert.equal(readProgressEvent(undefined).computable, false)
})

test('reporter：传输中报 transferring，字节发满自动切 processing', () => {
  const events = []
  const reporter = makeUploadProgressReporter('a.mp4', (p) => events.push(p))
  reporter({ loaded: 40 * MB, total: 100 * MB })
  reporter({ loaded: 100 * MB, total: 100 * MB })

  assert.equal(events[0].percent, 40)
  assert.equal(events[0].phase, 'transferring')
  assert.equal(events[0].file, 'a.mp4')
  assert.equal(events[1].percent, TRANSFER_CEIL)
  assert.equal(events[1].phase, 'processing')
})

test('reporter：total 拿不到时百分比为 0（不编造假进度）', () => {
  const events = []
  makeUploadProgressReporter('a.mp4', (p) => events.push(p))({ loaded: 40 * MB })
  assert.equal(events[0].percent, 0)
  assert.equal(events[0].total, 0)
  assert.equal(events[0].phase, 'transferring')
})

test('reporter：上层没传 onProgress 也不会炸', () => {
  assert.doesNotThrow(() => makeUploadProgressReporter('a.mp4', undefined)({ loaded: 1, total: 2 }))
})

test('progressText：阶段 · 文件 · 已传/总量', () => {
  assert.equal(
    progressText({ phase: 'uploading', file: 'big.mp4', loaded: 10 * MB, total: 100 * MB }),
    '分片直传中 · big.mp4 · 10.0MB / 100.0MB',
  )
})

test('progressText：缺字段就不拼缺的那段', () => {
  assert.equal(progressText({ phase: 'transferring' }), '上传中')
  assert.equal(progressText({ file: 'a.pdf', loaded: 1024, total: 4096 }), 'a.pdf · 1.0KB / 4.0KB')
  assert.equal(progressText({}), '')
})

test('phaseLabel：没见过的阶段退回"上传中"而不是显示英文', () => {
  assert.equal(phaseLabel('merging'), '服务端合并分片中')
  assert.equal(phaseLabel('weird'), '上传中')
  assert.equal(phaseLabel(undefined), '上传中')
})

test('queued 阶段说清楚"字节传完了，转码还在排队"', () => {
  assert.equal(phaseLabel('queued'), '已提交，服务器排队转码中')
})

test('formatBytes：B 不带小数，其余一位小数', () => {
  assert.equal(formatBytes(0), '0B')
  assert.equal(formatBytes(999), '999B')
  assert.equal(formatBytes(1024), '1.0KB')
  assert.equal(formatBytes(5 * MB), '5.0MB')
  assert.equal(formatBytes(3 * 1024 * MB), '3.0GB')
  assert.equal(formatBytes(null), '-')
  assert.equal(formatBytes(-1), '-')
})

test('applyProgressEvent：空字段不覆盖旧值', () => {
  const state = newProgressState()
  applyProgressEvent(state, { percent: 30, phase: 'uploading', file: 'a.mp4', loaded: 30, total: 100 })
  applyProgressEvent(state, { percent: 40 })
  assert.equal(state.percent, 40)
  assert.equal(state.phase, 'uploading')
  assert.equal(state.file, 'a.mp4')
})

test('applyProgressEvent：null 参数不炸', () => {
  const state = newProgressState()
  assert.equal(applyProgressEvent(state, null), state)
  assert.equal(applyProgressEvent(null, { percent: 1 }), null)
})

test('resetProgressState：清零，重开弹窗不会看到上一次的 100%', () => {
  const state = newProgressState()
  applyProgressEvent(state, { percent: 100, phase: 'done', file: 'a.mp4', loaded: 9, total: 9 })
  resetProgressState(state)
  assert.deepEqual(state, newProgressState())
})

test('progressLine 直接吃状态对象', () => {
  const state = newProgressState()
  applyProgressEvent(state, { percent: 10, phase: 'uploading', file: 'x', loaded: 512, total: 4096 })
  assert.equal(progressLine(state), '分片直传中 · x · 512B / 4.0KB')
  assert.equal(progressLine(undefined), '')
})

test('batchLabel：单个文件没有标签（用文件名自己顶），多个才写"N 个视频"', () => {
  assert.equal(batchLabel(3, '视频'), '3 个视频')
  assert.equal(batchLabel(1, '视频'), '')
  assert.equal(batchLabel(0), '')
})

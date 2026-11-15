// 刷新保留进度：job_id 生成 + localStorage 快照往返 的纯函数单测。
// 这些函数是「断点续传」的前端基础：刷新 / 关页后据此恢复进度 / 题目。
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  newJobId,
  saveGenerateJob,
  loadGenerateJob,
  clearGenerateJob,
  loadRecords,
  saveRecord,
  saveRecords,
  deleteRecord,
  clearRecords,
  buildRecord,
  recordSummary,
  saveViewing,
  loadViewing,
} from '../src/utils/aiGenerate.js'

function mockLocalStorage() {
  const m = new Map()
  globalThis.localStorage = {
    getItem: (k) => (m.has(k) ? m.get(k) : null),
    setItem: (k, v) => m.set(k, String(v)),
    removeItem: (k) => m.delete(k),
  }
  return m
}

test('newJobId 返回唯一且非空字符串', () => {
  mockLocalStorage()
  const a = newJobId()
  const b = newJobId()
  assert.equal(typeof a, 'string')
  assert.ok(a.length > 0)
  assert.notEqual(a, b)
})

test('save/load/clear 进度快照可在 localStorage 往返', () => {
  mockLocalStorage()
  const snap = {
    jobId: 'j1',
    payload: { knowledge: '光电效应', job_id: 'j1', count: 3 },
    progress: { stages: [] },
    meta: { attempts: 1 },
    questions: [{ stem: 'q1' }],
    elapsed: 12,
    done: false,
    t: Date.now(),
  }
  saveGenerateJob(snap)
  const got = loadGenerateJob()
  assert.equal(got.jobId, 'j1')
  assert.equal(got.payload.knowledge, '光电效应')
  assert.equal(got.questions[0].stem, 'q1')
  assert.equal(got.done, false)
  clearGenerateJob()
  assert.equal(loadGenerateJob(), null)
})

test('localStorage 不可用时静默降级不抛错', () => {
  // 不注入 localStorage，save/load/clear 都应吞掉异常
  delete globalThis.localStorage
  assert.doesNotThrow(() => {
    saveGenerateJob({ jobId: 'x' })
    loadGenerateJob()
    clearGenerateJob()
  })
  assert.equal(loadGenerateJob(), null)
})

// ===== 出题记录（历史）=====
test('buildRecord 复用 job_id 并打包题目与元信息', () => {
  mockLocalStorage()
  const payload = { knowledge: '截止频率', job_id: 'rec-1', count: 3, qtype: 'single' }
  const meta = { attempts: 1, material_status: 'ok', materials: [{}, {}] }
  const rec = buildRecord(payload, meta, [{ stem: 'a' }, { stem: 'b' }], 40)
  assert.equal(rec.id, 'rec-1')
  assert.equal(rec.knowledge, '截止频率')
  assert.equal(rec.qtype, 'single')
  assert.equal(rec.questions.length, 2)
  assert.equal(rec.elapsed, 40)
  assert.equal(rec.meta.materials.length, 2)
})

test('saveRecord 后 loadRecords 能取到，且最新在前、去重、超限截断', () => {
  mockLocalStorage()
  clearRecords()
  saveRecord(buildRecord({ job_id: 'a', knowledge: 'A' }, {}, [{ stem: '1' }], 10))
  saveRecord(buildRecord({ job_id: 'b', knowledge: 'B' }, {}, [{ stem: '2' }], 20))
  // 同 id 再次保存应去重（更新而非新增）
  saveRecord(buildRecord({ job_id: 'a', knowledge: 'A2' }, {}, [{ stem: '1' }, { stem: '3' }], 30))
  const list = loadRecords()
  assert.equal(list.length, 2)
  assert.equal(list[0].id, 'a') // 最新的在前
  assert.equal(list[0].knowledge, 'A2')

  // 超限（>50）截断
  for (let i = 0; i < 60; i++) saveRecord(buildRecord({ job_id: 'x' + i }, {}, [], i))
  assert.ok(loadRecords().length <= 50)
})

test('deleteRecord 移除指定记录，clearRecords 清空全部', () => {
  mockLocalStorage()
  clearRecords()
  saveRecord(buildRecord({ job_id: 'a' }, {}, [], 1))
  saveRecord(buildRecord({ job_id: 'b' }, {}, [], 2))
  const afterDel = deleteRecord('a')
  assert.deepEqual(afterDel.map((r) => r.id), ['b'])
  clearRecords()
  assert.deepEqual(loadRecords(), [])
})

test('recordSummary 给出知识点 + 题数 + 时间摘要', () => {
  mockLocalStorage()
  const rec = buildRecord({ job_id: 'r', knowledge: '光电效应' }, {}, [{}, {}, {}], 12)
  const s = recordSummary(rec)
  assert.match(s, /光电效应/)
  assert.match(s, /3 题/)
})

test('buildRecord 留存原始 payload，供「未完成」记录重试按原参数重发', () => {
  mockLocalStorage()
  const payload = {
    job_id: 'j1', knowledge: '假设检验', qtype: 'single', count: 3, score: 5,
    source: 'ai', subject: '数学', difficulty: '中等', grounding: true,
    seed: 's1', exclude_stems: ['旧题一'],
  }
  const rec = buildRecord(payload, {}, [], 0, 'interrupted', null)
  // 整包留存且是副本（改记录不影响原 payload）
  assert.equal(rec.payload.knowledge, '假设检验')
  assert.equal(rec.payload.count, 3)
  assert.deepEqual(rec.payload.exclude_stems, ['旧题一'])
  rec.payload.count = 99
  assert.equal(payload.count, 3)
  // 老式调用（无 payload 参数）也不炸
  const legacy = buildRecord({}, {}, [], 0, 'done', null)
  assert.deepEqual(legacy.payload, {})
})

// ===== 任务记录状态：进行中的任务刷新不丢 =====
test('点「生成」即落 running 记录，失败转 interrupted，完成转 done（同 id 更新不重复）', () => {
  mockLocalStorage()
  clearRecords()
  const payload = { knowledge: 'Go 语言', job_id: 'task-1', count: 3, qtype: 'single' }
  // 1) 生成一开始就落一条 running 记录：即使只跑到前两步就刷新，记录也在
  saveRecord(buildRecord(payload, {}, [], 0, 'running', { stages: [] }))
  let list = loadRecords()
  assert.equal(list.length, 1)
  assert.equal(list[0].status, 'running')
  assert.match(recordSummary(list[0]), /生成中/)
  // 2) 中断/失败：同 id 更新为 interrupted，不新增记录
  saveRecord(buildRecord(payload, {}, [], 5, 'interrupted', { stages: [] }))
  list = loadRecords()
  assert.equal(list.length, 1)
  assert.equal(list[0].status, 'interrupted')
  assert.match(recordSummary(list[0]), /未完成/)
  // 3) 完成：同 id 更新为 done，带完整题目
  saveRecord(buildRecord(payload, { attempts: 2 }, [{ stem: 'a' }, { stem: 'b' }, { stem: 'c' }], 81, 'done'))
  list = loadRecords()
  assert.equal(list.length, 1)
  assert.equal(list[0].status, 'done')
  assert.equal(list[0].questions.length, 3)
  assert.match(recordSummary(list[0]), /3 题/)
})

test('saveRecords 批量写回：刷新时可把残留 running 降级为 interrupted', () => {
  mockLocalStorage()
  clearRecords()
  // 模拟一条已完成的 + 一条残留 running 的（例如浏览器被强杀后留下的）
  saveRecord(buildRecord({ job_id: 'done-1', knowledge: 'A' }, {}, [{ stem: '1' }], 10, 'done'))
  saveRecord(buildRecord({ job_id: 'run-1', knowledge: 'B' }, {}, [], 3, 'running'))
  const demoted = loadRecords().map((r) => (r.status === 'running' ? { ...r, status: 'interrupted' } : r))
  const saved = saveRecords(demoted)
  assert.equal(saved.length, 2)
  const list = loadRecords()
  assert.equal(list.find((r) => r.id === 'done-1').status, 'done')
  assert.equal(list.find((r) => r.id === 'run-1').status, 'interrupted')
  // 超限截断
  const big = []
  for (let i = 0; i < 60; i++) big.push(buildRecord({ job_id: 'y' + i }, {}, [], i, 'interrupted'))
  assert.equal(saveRecords(big).length, 50)
})

test('saveViewing / loadViewing 往返', () => {
  mockLocalStorage()
  saveViewing('rec-1')
  assert.equal(loadViewing(), 'rec-1')
  saveViewing(null)
  assert.equal(loadViewing(), null)
})

// 大文件分片直传 + 断点续传的单元测试（Node 内置测试运行器，不碰浏览器也不碰 MinIO）。
//
// 重点锁的不是"能不能跑通"，而是几个只有出错才知道贵的点：
//   - 半截分片（大小不符）绝不能被当成已传完，否则合出坏视频；
//   - 服务端状态不可信时必须重新 init，不能拿旧 uploadId 接着传；
//   - 取消要既清本地记账、又通知服务端归还额度；
//   - 成功一片就落一次记账（刷新后续传才可能只补缺的）。

import { test } from 'node:test'
import assert from 'node:assert/strict'

import {
  CHUNK_SIZE,
  MULTIPART_THRESHOLD,
  MAX_FILE_BYTES,
  shouldUseMultipart,
  chunkPlan,
  partRange,
  completedParts,
  missingParts,
  resumeKey,
  saveResume,
  loadResume,
  clearResume,
  uploadByMultipart,
  UploadCancelled,
} from '../src/utils/videoUpload.js'

const MB = 1024 * 1024

// 最小 File 替身：上传逻辑只读 name / size / lastModified / slice
function fakeFile(name, size, lastModified = 1700000000000) {
  return {
    name,
    size,
    lastModified,
    slice(start, end) {
      return { start, end, size: end - start }
    },
  }
}

// 可编程的 api 替身：默认全部成功，调用参数全部记录下来
function makeApi(overrides = {}) {
  const calls = { init: [], sign: [], complete: [], abort: [], status: [] }
  const api = {
    initMultipartUpload: async (data) => {
      calls.init.push(data)
      return { data: { uploadId: 'up-1', objectName: 'course/9/videos/x.mp4', chunkSize: CHUNK_SIZE, minPartSize: CHUNK_SIZE } }
    },
    signMultipartParts: async ({ partNumbers }) => {
      calls.sign.push(partNumbers.slice())
      return {
        data: {
          expireIn: 900,
          parts: partNumbers.map((n) => ({ partNumber: n, url: `http://minio/part/${n}` })),
        },
      }
    },
    completeMultipartUpload: async (data) => {
      calls.complete.push(data)
      return { data: { courseId: 9, video: { id: 3, name: 'big.mp4' } } }
    },
    abortMultipartUpload: async (data) => {
      calls.abort.push(data)
      return { data: { message: '已取消' } }
    },
    fetchMultipartUploadStatus: async (data) => {
      calls.status.push(data)
      return { data: { status: 'not_found', resumable: false, parts: [] } }
    },
  }
  for (const [k, v] of Object.entries(overrides)) api[k] = v
  return { api, calls }
}

// 记录 PUT 的分片；failOnce 里的片号第一次失败
function makePut({ failOnce = new Set() } = {}) {
  const putParts = []
  const seen = new Set()
  const put = async (url, blob) => {
    const n = Number(String(url).split('/').pop())
    putParts.push(n)
    if (failOnce.has(n) && !seen.has(n)) {
      seen.add(n)
      throw new Error('network reset')
    }
    return { ok: true }
  }
  return { put, putParts }
}

function memStorage() {
  const store = new Map()
  return {
    getItem: (k) => (store.has(k) ? store.get(k) : null),
    setItem: (k, v) => store.set(k, String(v)),
    removeItem: (k) => store.delete(k),
    dump: () => Object.fromEntries(store),
  }
}

const SIZE = 105 * MB // 21 片：20 整片 + 1 片 5MB 整除边界（105/5=21，正好齐）
const EXACT = 20 * MB // 4 片，无尾片

// ===== 纯函数 =====

test('shouldUseMultipart：阈值上下各一档', () => {
  assert.equal(MULTIPART_THRESHOLD, 250 * MB)
  assert.equal(shouldUseMultipart({ size: 250 * MB }), false, '刚好 250MB 仍走表单上传')
  assert.equal(shouldUseMultipart({ size: 250 * MB + 1 }), true)
  assert.equal(shouldUseMultipart({ size: 800 * MB }), true)
  assert.equal(shouldUseMultipart(undefined), false)
})

test('单文件硬上限 800MB：刚好压线可以传，切成 160 片', async () => {
  assert.equal(MAX_FILE_BYTES, 800 * MB)
  const { api } = makeApi()
  const { put, putParts } = makePut()
  await uploadByMultipart(fakeFile('max.mp4', MAX_FILE_BYTES), {
    courseId: 9,
    api,
    storage: memStorage(),
    put,
  })
  assert.equal(putParts.length, 160, '800MB / 5MB = 160 片')
})

test('超过 800MB：一个字节都不发就报错，文案带文件名和上限', async () => {
  const { api, calls } = makeApi()
  const { put, putParts } = makePut()
  await assert.rejects(
    uploadByMultipart(fakeFile('huge.mp4', 800 * MB + 1), {
      courseId: 9,
      api,
      storage: memStorage(),
      put,
    }),
    /huge\.mp4.*800MB 上限/,
  )
  // 拦在 init 之前：不该建会话、不该占用户当天配额、更不该传分片
  assert.equal(calls.init.length, 0)
  assert.equal(calls.sign.length, 0)
  assert.equal(putParts.length, 0)
})

test('chunkPlan / partRange：最后一片不满', () => {
  assert.deepEqual(chunkPlan(11 * MB), { chunkSize: CHUNK_SIZE, total: 3 })
  assert.equal(partRange(1, 11 * MB).size, 5 * MB)
  assert.equal(partRange(3, 11 * MB).size, 1 * MB, '末片只剩 1MB')
  assert.deepEqual({ ...partRange(3, 11 * MB) }, { start: 10 * MB, end: 11 * MB, size: 1 * MB })
  // 越界片号不该算出负数
  assert.equal(partRange(9, 11 * MB).size, 0)
  // 0 字节文件也至少有 1 片（调用方已在前面挡住空文件）
  assert.equal(chunkPlan(0).total, 1)
})

test('completedParts：大小必须精确匹配，半截分片不算传完', () => {
  const size = 11 * MB
  const done = completedParts(
    [
      { partNumber: 1, size: 5 * MB },
      { partNumber: 2, size: 4 * MB }, // 半截：应是 5MB
      { partNumber: 3, size: 1 * MB },
      { partNumber: 99, size: 1 * MB }, // 越界片号
    ],
    size,
  )
  assert.deepEqual([...done].sort(), [1, 3], '第 2 片大小不符必须剔除')
  assert.deepEqual(missingParts([{ partNumber: 1, size: 5 * MB }], size), [2, 3])
})

test('resumeKey：换文件或换课程都不同 key', () => {
  const a = resumeKey(9, fakeFile('a.mp4', 100, 1))
  assert.notEqual(a, resumeKey(9, fakeFile('b.mp4', 100, 1)))
  assert.notEqual(a, resumeKey(10, fakeFile('a.mp4', 100, 1)))
  assert.notEqual(a, resumeKey(9, fakeFile('a.mp4', 101, 1)))
  assert.match(a, /^lp-video-upload:9:/)
})

test('记账读写对损坏内容容错', () => {
  const s = memStorage()
  saveResume(s, 'k', { uploadId: 'u' })
  assert.deepEqual(loadResume(s, 'k'), { uploadId: 'u' })
  s.setItem('k', '{不是 JSON')
  assert.equal(loadResume(s, 'k'), null)
  clearResume(s, 'k')
  assert.equal(loadResume(s, 'k'), null)
})

// ===== 上传流程 =====

test('整片上传：21 片全部直传后调 complete', async () => {
  const { api, calls } = makeApi()
  const { put, putParts } = makePut()
  const storage = memStorage()
  const file = fakeFile('big.mp4', SIZE)

  const out = await uploadByMultipart(file, { courseId: 9, api, storage, put })

  assert.equal(out.video.id, 3)
  assert.equal(calls.init.length, 1)
  assert.equal(calls.abort.length, 0)
  assert.equal(putParts.length, 21, `应有 21 片直传，实际 ${putParts.length}`)
  assert.deepEqual([...putParts].sort((a, b) => a - b), Array.from({ length: 21 }, (_, i) => i + 1))
  // complete 带上的分片清单必须覆盖 1..21，且总字节 = 文件大小
  const parts = calls.complete[0].parts
  assert.equal(parts.length, 21)
  assert.equal(parts.reduce((s, p) => s + p.size, 0), SIZE)
  // 成功后记账清干净，避免下次同名文件误续传
  assert.deepEqual(storage.dump(), {})
})

test('断点续传：服务端已有 1..8 片时只补缺的', async () => {
  const { api, calls } = makeApi({
    fetchMultipartUploadStatus: async () => ({
      data: {
        resumable: true,
        status: 'init',
        fileSize: SIZE,
        fileName: 'big.mp4',
        chunkSize: CHUNK_SIZE,
        parts: Array.from({ length: 8 }, (_, i) => ({ partNumber: i + 1, size: CHUNK_SIZE })),
      },
    }),
  })
  const { put, putParts } = makePut()
  const storage = memStorage()
  const key = resumeKey(9, fakeFile('big.mp4', SIZE))
  saveResume(storage, key, { uploadId: 'up-1', chunkSize: CHUNK_SIZE })

  await uploadByMultipart(fakeFile('big.mp4', SIZE), { courseId: 9, api, storage, put })

  assert.equal(calls.init.length, 0, '能续传就不该重新 init')
  assert.deepEqual([...putParts].sort((a, b) => a - b), Array.from({ length: 13 }, (_, i) => i + 9))
  assert.equal(calls.complete[0].uploadId, 'up-1')
})

test('会话过期/文件不符：丢弃旧 uploadId 重新 init', async () => {
  for (const status of [
    { resumable: false, status: 'expired', parts: [] },
    { resumable: true, status: 'init', fileSize: 999, fileName: 'big.mp4', parts: [] },
    { resumable: true, status: 'init', fileSize: SIZE, fileName: '其他.mp4', parts: [] },
  ]) {
    const { api, calls } = makeApi({ fetchMultipartUploadStatus: async () => ({ data: status }) })
    const { put } = makePut()
    const storage = memStorage()
    saveResume(storage, resumeKey(9, fakeFile('big.mp4', SIZE)), { uploadId: 'up-old', chunkSize: CHUNK_SIZE })

    await uploadByMultipart(fakeFile('big.mp4', SIZE), { courseId: 9, api, storage, put })

    assert.equal(calls.init.length, 1, `${JSON.stringify(status)} 应该重新 init`)
    assert.equal(calls.complete[0].uploadId, 'up-1', '合并要用新的 uploadId')
  }
})

test('单片失败后重试成功，整轮不中断', async () => {
  const { api, calls } = makeApi()
  const { put, putParts } = makePut({ failOnce: new Set([7]) })
  await uploadByMultipart(fakeFile('big.mp4', SIZE), { courseId: 9, api, storage: memStorage(), put })

  assert.equal(calls.complete.length, 1)
  assert.equal(putParts.filter((n) => n === 7).length, 2, '第 7 片应恰好重传一次')
})

test('取消上传：通知服务端 abort 并清本地记账', async () => {
  const { api, calls } = makeApi()
  const storage = memStorage()
  const controller = new AbortController()
  let putCount = 0
  const put = async (url) => {
    putCount += 1
    if (putCount >= 2) controller.abort()
    return { ok: true }
  }

  await assert.rejects(
    uploadByMultipart(fakeFile('big.mp4', SIZE), { courseId: 9, api, storage, put, signal: controller.signal }),
    (err) => err instanceof UploadCancelled,
  )
  assert.equal(calls.abort.length, 1, '取消必须让服务端删分片并归还额度')
  assert.equal(calls.complete.length, 0)
  assert.deepEqual(storage.dump(), {}, '取消后不能留下可续传的记账')
})

test('合并失败时保留记账，让用户能重试', async () => {
  const { api } = makeApi({
    completeMultipartUpload: async () => {
      throw Object.assign(new Error('boom'), { response: { data: { msg: '第 3 片缺失' } } })
    },
  })
  const { put } = makePut()
  const storage = memStorage()

  await assert.rejects(
    uploadByMultipart(fakeFile('big.mp4', SIZE), { courseId: 9, api, storage, put }),
    /第 3 片缺失/,
  )
  assert.ok(Object.keys(storage.dump()).length === 1, '合并失败要留下续传记账')
})

test('进度回调：单调不回退，末态 100%', async () => {
  const { api } = makeApi()
  const { put } = makePut()
  const seen = []
  await uploadByMultipart(fakeFile('big.mp4', EXACT), {
    courseId: 9,
    api,
    storage: memStorage(),
    put,
    onProgress: (p) => seen.push(p),
  })

  for (let i = 1; i < seen.length; i += 1) {
    assert.ok(seen[i].loaded >= seen[i - 1].loaded, `loaded 回退了：${seen[i - 1].loaded} → ${seen[i].loaded}`)
  }
  // 进度条状态盒只认 loaded/total（见 uploadProgress.js applyProgressEvent），
  // 字段名一旦改回 uploaded，分片上传的字节数就会永远显示 0B
  assert.ok(seen.every((p) => typeof p.loaded === 'number' && typeof p.total === 'number'))
  assert.equal(seen[seen.length - 1].phase, 'done')
  assert.equal(seen[seen.length - 1].percent, 100)
})

test('空文件直接拒绝', async () => {
  const { api, calls } = makeApi()
  await assert.rejects(
    uploadByMultipart(fakeFile('empty.mp4', 0), { courseId: 9, api, storage: memStorage(), put: async () => ({ ok: true }) }),
    /空文件/,
  )
  assert.equal(calls.init.length, 0, '不该为空文件建会话（会白占配额）')
})

test('签名批次按窗口切，且每片只签一次', async () => {
  const { api, calls } = makeApi()
  const { put } = makePut()
  await uploadByMultipart(fakeFile('big.mp4', SIZE), { courseId: 9, api, storage: memStorage(), put, windowSize: 10 })

  const all = calls.sign.flat()
  assert.equal(all.length, 21)
  assert.equal(new Set(all).size, 21, '片号不该重复签')
  assert.deepEqual(calls.sign.map((g) => g.length), [10, 10, 1])
})

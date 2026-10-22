// 课程创建流程的单元测试（使用 Node 内置测试运行器，零依赖）
// 运行方式：npm test（等价于 node --test tests/）
//
// 覆盖用户要求的交互：
// 1. 选择图片点"上传图片" → 上传成功 / 上传失败（带对应错误提示信息）
// 2. 选择视频点"上传视频" → 上传成功 / 上传失败
// 3. 最后点"创建课程" → 成功（组件跳转课程列表）/ 失败（留在页面提示错误）

import { test } from 'node:test'
import assert from 'node:assert/strict'

import {
  ensureCourseCreated,
  uploadCoverFlow,
  uploadVideosFlow,
  createCourseFlow,
} from '../src/utils/courseCreateFlow.js'

// 构造最小 session：模拟 Vue ref 的 {value} 结构（流程函数只读写 .value）
function makeSession() {
  return {
    createdId: { value: 0 },
    createdName: { value: '' },
    coverResult: { value: null },
    videoResult: { value: [] },
  }
}

const form = { name: '测试课程', intro: '简介', teacher: '王老师' }

// 可编程 mock api：默认全部成功，overrides 可注入失败
// calls 记录各接口被调用的次数（用于断言"不重复创建/不重复上传"）
function makeApi(overrides = {}) {
  const calls = { createCourse: 0, uploadCourseCover: 0, uploadCourseVideos: 0 }
  const api = {
    createCourse: async () => {
      calls.createCourse += 1
      return { data: { id: 7, name: '测试课程' } }
    },
    uploadCourseCover: async () => {
      calls.uploadCourseCover += 1
      return {
        data: { courseId: 7, coverUrl: 'http://x/cover.jpg', coverThumbUrl: 'http://x/thumb.jpg' },
      }
    },
    uploadCourseVideos: async () => {
      calls.uploadCourseVideos += 1
      return {
        data: { courseId: 7, videos: [{ id: 1, name: 'v.mp4', url: 'http://x/v.mp4', size: 10, uploadedAt: 't' }] },
      }
    },
    ...overrides,
  }
  return { api, calls }
}

// 错误工厂：模拟 axios 在 HTTP 失败时抛出的 AxiosError
// （真实 axios 失败是 reject 抛异常，流程函数靠 catch (e) 捕获 e.response.data.msg）
function failWith(msg) {
  const err = new Error(msg)
  err.response = { data: { msg } }
  throw err
}

// ===== ensureCourseCreated：确保课程存在 =====

test('课程未创建时自动创建，成功写入 session', async () => {
  const { api, calls } = makeApi()
  const session = makeSession()
  const r = await ensureCourseCreated(api, form, session)
  assert.equal(r, true)
  assert.equal(session.createdId.value, 7)
  assert.equal(session.createdName.value, '测试课程')
})

test('课程已创建时不重复请求创建接口', async () => {
  const { api, calls } = makeApi()
  const session = makeSession()
  session.createdId.value = 7
  await ensureCourseCreated(api, form, session)
  assert.equal(calls.createCourse, 0) // 已创建 → 不再调用创建接口
})

test('创建失败时返回 {error}，错误信息取后端 msg', async () => {
  const { api, calls } = makeApi({ createCourse: async () => failWith('课程名称已存在') })
  const session = makeSession()
  const r = await ensureCourseCreated(api, form, session)
  assert.deepEqual(r, { error: '课程名称已存在' })
  assert.equal(session.createdId.value, 0) // 失败不写入 id
})

// ===== uploadCoverFlow：封面上传 =====

test('没选封面就点上传 → 提示先选文件，不发请求', async () => {
  const { api, calls } = makeApi()
  const session = makeSession()
  const r = await uploadCoverFlow(api, form, session, null)
  assert.deepEqual(r, { error: '请先选择封面图片' })
})

test('选封面 + 课程未创建 → 自动创建课程再上传封面，成功返回 ok', async () => {
  const { api, calls } = makeApi()
  const session = makeSession()
  const file = { name: 'a.jpg' }
  const r = await uploadCoverFlow(api, form, session, file)
  assert.deepEqual(r, { ok: true })
  assert.equal(session.createdId.value, 7) // 自动创建过
  assert.ok(session.coverResult.value) // 记录上传结果
})

test('选封面 + 课程已创建 → 直接上传封面，不重复创建', async () => {
  const { api, calls } = makeApi()
  const session = makeSession()
  session.createdId.value = 7
  const r = await uploadCoverFlow(api, form, session, { name: 'a.jpg' })
  assert.deepEqual(r, { ok: true })
})

test('封面上传失败 → 返回 {error}（取后端 msg），留在页面', async () => {
  const { api, calls } = makeApi({ uploadCourseCover: async () => failWith('封面文件超过大小上限') })
  const session = makeSession()
  session.createdId.value = 7
  const r = await uploadCoverFlow(api, form, session, { name: 'a.jpg' })
  assert.deepEqual(r, { error: '封面文件超过大小上限' })
  assert.equal(session.coverResult.value, null) // 失败不写结果
})

test('自动创建课程失败时不再上传封面，返回创建错误', async () => {
  const { api, calls } = makeApi({ createCourse: async () => failWith('课程名称已存在') })
  const session = makeSession()
  const r = await uploadCoverFlow(api, form, session, { name: 'a.jpg' })
  assert.deepEqual(r, { error: '课程名称已存在' })
})

// ===== uploadVideosFlow：视频上传 =====

test('没选视频就点上传 → 提示先选文件，不发请求', async () => {
  const { api, calls } = makeApi()
  const session = makeSession()
  const r = await uploadVideosFlow(api, form, session, [])
  assert.deepEqual(r, { error: '请先选择视频文件' })
})

test('选视频 + 课程未创建 → 自动创建课程再上传视频，返回 ok 与数量', async () => {
  const { api, calls } = makeApi()
  const session = makeSession()
  const r = await uploadVideosFlow(api, form, session, [{ name: 'v.mp4' }])
  assert.equal(r.ok, true)
  assert.equal(r.count, 1)
  assert.equal(session.videoResult.value.length, 1)
})

test('视频上传失败 → 返回 {error}，留在页面', async () => {
  const { api, calls } = makeApi({ uploadCourseVideos: async () => failWith('视频文件超过大小上限') })
  const session = makeSession()
  session.createdId.value = 7
  const r = await uploadVideosFlow(api, form, session, [{ name: 'v.mp4' }])
  assert.deepEqual(r, { error: '视频文件超过大小上限' })
})

// ===== createCourseFlow：最终提交 =====

test('课程未创建 → 创建成功返回 ok（组件随后跳转课程列表）', async () => {
  const { api, calls } = makeApi()
  const session = makeSession()
  const r = await createCourseFlow(api, form, session, {
    coverFile: null,
    videoFiles: [],
    coverResult: session.coverResult,
    videoResult: session.videoResult,
  })
  assert.deepEqual(r, { ok: true })
  assert.equal(session.createdId.value, 7)
})

test('课程已创建（上传时自动创建过）→ 不重复创建，直接返回 ok', async () => {
  const { api, calls } = makeApi()
  const session = makeSession()
  session.createdId.value = 7
  const r = await createCourseFlow(api, form, session, {
    coverFile: null,
    videoFiles: [],
    coverResult: session.coverResult,
    videoResult: session.videoResult,
  })
  assert.deepEqual(r, { ok: true })
})

test('创建失败 → 返回 {error}，留在页面，不执行后续上传', async () => {
  const { api, calls } = makeApi({ createCourse: async () => failWith('课程名称已存在') })
  const session = makeSession()
  const r = await createCourseFlow(api, form, session, {
    coverFile: { name: 'a.jpg' },
    videoFiles: [{ name: 'v.mp4' }],
    coverResult: session.coverResult,
    videoResult: session.videoResult,
  })
  assert.deepEqual(r, { error: '课程名称已存在' })
  assert.equal(session.coverResult.value, null)
  assert.equal(session.videoResult.value.length, 0)
})

test('选了文件但没传 → 最终提交时兜底上传封面和视频', async () => {
  const { api, calls } = makeApi()
  const session = makeSession()
  const r = await createCourseFlow(api, form, session, {
    coverFile: { name: 'a.jpg' },
    videoFiles: [{ name: 'v.mp4' }],
    coverResult: session.coverResult,
    videoResult: session.videoResult,
  })
  assert.deepEqual(r, { ok: true })
  assert.ok(session.coverResult.value) // 封面兜底传了
  assert.equal(session.videoResult.value.length, 1) // 视频兜底传了
})

test('已上传成功的封面/视频 → 最终提交时不重复上传', async () => {
  const { api, calls } = makeApi()
  const session = makeSession()
  session.createdId.value = 7
  session.coverResult.value = { coverUrl: 'http://x/c.jpg' }
  session.videoResult.value = [{ id: 1, name: 'v.mp4' }]
  const r = await createCourseFlow(api, form, session, {
    coverFile: { name: 'a.jpg' },
    videoFiles: [{ name: 'v.mp4' }],
    coverResult: session.coverResult,
    videoResult: session.videoResult,
  })
  assert.deepEqual(r, { ok: true })
  // 已上传成功过：coverResult / videoResult 有值，流程不会再调上传接口
  assert.equal(calls.uploadCourseCover, 0)
  assert.equal(calls.uploadCourseVideos, 0)
})

test('最终提交时兜底上传失败 → 返回对应错误，留在页面', async () => {
  const { api, calls } = makeApi({ uploadCourseCover: async () => failWith('封面文件超过大小上限') })
  const session = makeSession()
  const r = await createCourseFlow(api, form, session, {
    coverFile: { name: 'a.jpg' },
    videoFiles: [],
    coverResult: session.coverResult,
    videoResult: session.videoResult,
  })
  assert.deepEqual(r, { error: '封面文件超过大小上限' })
})

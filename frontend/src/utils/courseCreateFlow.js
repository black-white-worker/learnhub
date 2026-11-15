// 课程创建页的流程逻辑（独立成纯函数，便于单元测试）
//
// 后端上传接口（/api/course/upload/cover、/api/course/upload/videos）
// 要求课程已存在（按 courseId 定位课程），所以"上传封面/上传视频"
// 按钮点击时会先自动创建课程（课程不存在才创建），保证上传按钮
// 可以独立使用；"创建课程"作为最终提交，成功后跳转课程列表，
// 失败留在本页提示对应错误。

import { shouldUseMultipart, uploadByMultipart } from './videoUpload.js'
import { makeUploadProgressReporter, reportDone, batchLabel } from './uploadProgress.js'

// 确保课程存在：不存在则用表单数据创建，成功返回 true，失败返回 {error}
export async function ensureCourseCreated(api, form, session) {
  if (session.createdId.value) return true
  try {
    const res = await api.createCourse({
      name: form.name,
      intro: form.intro,
      teacher: form.teacher,
      createdBy: localStorage.getItem('username') || '',
    })
    session.createdId.value = res.data.id
    session.createdName.value = res.data.name
    return true
  } catch (e) {
    return { error: e.response?.data?.msg || e.message || '创建课程失败，请重试' }
  }
}

// 封面：校验已选文件 → 校验课程名 → 确保课程存在 → 上传封面 → 记录结果
// 成功返回 {ok:true}，失败返回 {error}
export async function uploadCoverFlow(api, form, session, file, opts = {}) {
  if (!file) return { error: '请先选择封面图片' }
  // 未填课程名就点上传封面：直接友好提示，不发请求（避免后端报一堆英文错误）
  if (!form.name || !form.name.trim()) return { error: '请先输入课程名称，再上传封面' }
  const ensured = await ensureCourseCreated(api, form, session)
  if (ensured !== true) return ensured
  const fd = new FormData()
  fd.append('file', file)
  try {
    const res = await api.uploadCourseCover(
      session.createdId.value,
      fd,
      makeUploadProgressReporter(file.name, opts.onProgress),
    )
    session.coverResult.value = res.data
    // 原图 + 缩略图都已落库并回了包，这时候才允许走到 100%
    reportDone(opts.onProgress, { file: file.name })
    return { ok: true }
  } catch (e) {
    return { error: e.response?.data?.msg || e.message || '封面上传失败，请重试' }
  }
}

// 视频：校验已选文件 → 校验课程名 → 确保课程存在 → 上传视频 → 记录结果
// 成功返回 {ok:true, count}，失败返回 {error}
//
// 大文件（>250MB）不能塞进同一个 FormData：一次 POST 几百 MB，
// 中途断一下就得整批重传，而且后端中转会把容器内存吃满。
// 所以这里按大小分流：小文件维持表单批量上传，大文件逐个走分片直传。
export async function uploadVideosFlow(api, form, session, files, opts = {}) {
  if (!files || files.length === 0) return { error: '请先选择视频文件' }
  if (!form.name || !form.name.trim()) return { error: '请先输入课程名称，再上传视频' }
  const ensured = await ensureCourseCreated(api, form, session)
  if (ensured !== true) return ensured

  const isBig = opts.shouldUseMultipart || shouldUseMultipart
  const putMultipart = opts.uploadMultipart || uploadByMultipart
  const small = files.filter((f) => !isBig(f))
  const big = files.filter((f) => isBig(f))

  const uploaded = []
  try {
    if (small.length > 0) {
      const fd = new FormData()
      small.forEach((v) => fd.append('files', v))
      // 一次请求带多个文件，进度天然是整批的，标签用"N 个视频"而不是某一个文件名
      const label = batchLabel(small.length, '视频') || small[0].name
      const res = await api.uploadCourseVideos(
        session.createdId.value,
        fd,
        makeUploadProgressReporter(label, opts.onProgress),
      )
      uploaded.push(...(res.data.videos || []))
      // 字节已落到对象存储、数据库记录已写；faststart 转码由后端队列异步做
      reportDone(opts.onProgress, { file: label, phase: 'queued' })
    }
    for (const file of big) {
      const res = await putMultipart(file, {
        courseId: session.createdId.value,
        api,
        // util 报 'done' 只代表字节合并完成，转码还在排队 —— 文案要说清楚
        onProgress: (p) =>
          opts.onProgress?.({ ...p, phase: p.phase === 'done' ? 'queued' : p.phase, file: file.name }),
      })
      // complete 接口返回单个视频；老实现/测试桩可能只给 videos
      if (res?.video) uploaded.push(res.video)
      else if (Array.isArray(res?.videos)) uploaded.push(...res.videos)
    }
  } catch (e) {
    if (e?.name === 'UploadCancelled') return { error: '视频上传已取消' }
    return { error: e.response?.data?.msg || e.message || '视频上传失败，请重试' }
  }

  session.videoResult.value = uploaded
  return { ok: true, count: uploaded.length }
}

// 最终提交（创建课程按钮）：
// 1. 确保课程存在（未创建则创建，已创建直接复用）
// 2. 兜底上传"选了文件但还没传成功"的封面/视频（兼容先选文件直接点创建的用法）
// 全部成功返回 {ok:true}，任一步失败返回 {error}，调用方留在页面提示
export async function createCourseFlow(api, form, session, pending, opts = {}) {
  const ensured = await ensureCourseCreated(api, form, session)
  if (ensured !== true) return ensured

  if (pending.coverFile && !pending.coverResult.value) {
    const r = await uploadCoverFlow(api, form, session, pending.coverFile, opts)
    if (r !== true && r.ok !== true) return r
  }
  if (pending.videoFiles.length > 0 && pending.videoResult.value.length === 0) {
    const r = await uploadVideosFlow(api, form, session, pending.videoFiles, opts)
    if (r !== true && r.ok !== true) return r
  }
  return { ok: true }
}

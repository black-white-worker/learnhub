// 课程创建页的流程逻辑（独立成纯函数，便于单元测试）
//
// 后端上传接口（/api/course/upload/cover、/api/course/upload/videos）
// 要求课程已存在（按 courseId 定位课程），所以"上传封面/上传视频"
// 按钮点击时会先自动创建课程（课程不存在才创建），保证上传按钮
// 可以独立使用；"创建课程"作为最终提交，成功后跳转课程列表，
// 失败留在本页提示对应错误。

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
export async function uploadCoverFlow(api, form, session, file) {
  if (!file) return { error: '请先选择封面图片' }
  // 未填课程名就点上传封面：直接友好提示，不发请求（避免后端报一堆英文错误）
  if (!form.name || !form.name.trim()) return { error: '请先输入课程名称，再上传封面' }
  const ensured = await ensureCourseCreated(api, form, session)
  if (ensured !== true) return ensured
  const fd = new FormData()
  fd.append('courseId', session.createdId.value)
  fd.append('file', file)
  try {
    const res = await api.uploadCourseCover(fd)
    session.coverResult.value = res.data
    return { ok: true }
  } catch (e) {
    return { error: e.response?.data?.msg || e.message || '封面上传失败，请重试' }
  }
}

// 视频：校验已选文件 → 校验课程名 → 确保课程存在 → 上传视频 → 记录结果
// 成功返回 {ok:true, count}，失败返回 {error}
export async function uploadVideosFlow(api, form, session, files) {
  if (!files || files.length === 0) return { error: '请先选择视频文件' }
  if (!form.name || !form.name.trim()) return { error: '请先输入课程名称，再上传视频' }
  const ensured = await ensureCourseCreated(api, form, session)
  if (ensured !== true) return ensured
  const fd = new FormData()
  fd.append('courseId', session.createdId.value)
  files.forEach((v) => fd.append('files', v))
  try {
    const res = await api.uploadCourseVideos(fd)
    session.videoResult.value = res.data.videos || []
    return { ok: true, count: session.videoResult.value.length }
  } catch (e) {
    return { error: e.response?.data?.msg || e.message || '视频上传失败，请重试' }
  }
}

// 最终提交（创建课程按钮）：
// 1. 确保课程存在（未创建则创建，已创建直接复用）
// 2. 兜底上传"选了文件但还没传成功"的封面/视频（兼容先选文件直接点创建的用法）
// 全部成功返回 {ok:true}，任一步失败返回 {error}，调用方留在页面提示
export async function createCourseFlow(api, form, session, pending) {
  const ensured = await ensureCourseCreated(api, form, session)
  if (ensured !== true) return ensured

  if (pending.coverFile && !pending.coverResult.value) {
    const r = await uploadCoverFlow(api, form, session, pending.coverFile)
    if (r !== true && r.ok !== true) return r
  }
  if (pending.videoFiles.length > 0 && pending.videoResult.value.length === 0) {
    const r = await uploadVideosFlow(api, form, session, pending.videoFiles)
    if (r !== true && r.ok !== true) return r
  }
  return { ok: true }
}

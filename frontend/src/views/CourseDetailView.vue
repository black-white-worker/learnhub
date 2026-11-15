<script setup>
import { ref, reactive, onMounted, onUnmounted, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { VideoPlay } from '@element-plus/icons-vue'
import {
  getCourseDetail,
  editCourse,
  deleteCourse,
  uploadCourseVideos,
  initMultipartUpload,
  signMultipartParts,
  completeMultipartUpload,
  abortMultipartUpload,
  fetchMultipartUploadStatus,
  reprocessCourseVideo,
} from '../api/index.js'
import CourseResourcePanel from '../components/CourseResourcePanel.vue'
import { shouldUseMultipart, uploadByMultipart } from '../utils/videoUpload.js'
import {
  DONE_PERCENT,
  applyProgressEvent,
  makeUploadProgressReporter,
  newProgressState,
  progressLine,
  resetProgressState,
} from '../utils/uploadProgress.js'
import {
  POLL_INTERVAL_MS,
  processStatusTag,
  canRetryProcess,
  hasRunningProcess,
} from '../utils/videoStatus.js'

const route = useRoute()
const router = useRouter()

const courseId = computed(() => Number(route.params.id))
const loading = ref(false)
const detail = ref(null)
const activeTab = ref('videos')

// 是否处于编辑模式（从列表页带 ?action=edit 进来，或点"编辑信息"按钮）
const editing = ref(route.query.action === 'edit')
const saving = ref(false)
const formRef = ref(null)

const form = reactive({ name: '', intro: '', teacher: '' })

const rules = {
  name: [
    { required: true, message: '课程名称不能为空', trigger: 'blur' },
    { min: 2, max: 50, message: '课程名称长度 2-50 个字符', trigger: 'blur' },
  ],
}

async function loadDetail() {
  loading.value = true
  try {
    const res = await getCourseDetail(courseId.value)
    detail.value = res.data
    // 同步到编辑表单
    form.name = detail.value.name
    form.intro = detail.value.intro || ''
    form.teacher = detail.value.teacher || ''
    syncPolling()
  } catch (e) {
    ElMessage.error('课程详情加载失败：' + errText(e))
  } finally {
    loading.value = false
  }
}

onMounted(loadDetail)

// 路由参数变化（同组件内切换课程）时重新加载
watch(courseId, () => {
  stopPolling()
  loadDetail()
})

onUnmounted(stopPolling)

// ===== 转码状态轮询 =====
//
// 后处理挪到后端队列之后，前端拿到的结果只是"排队中"，
// 老师需要看到它变成"已优化"，所以有在跑的任务时定时静默刷新。
// 用轮询而不是 WebSocket：详情页本来就有这个接口，不用为一个小徽标加长连接。
let pollTimer = null

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

// 只在还有 pending/processing 的视频时轮询，全部处理完自动停下
function syncPolling() {
  if (hasRunningProcess(detail.value?.videos)) startPolling()
  else stopPolling()
}

function startPolling() {
  if (pollTimer) return
  pollTimer = setInterval(refreshSilently, POLL_INTERVAL_MS)
}

// 静默刷新：不置 loading（否则每隔 5 秒整页闪一次遮罩），
// 也不覆盖编辑表单（老师正在改简介时不该被刷掉）。
async function refreshSilently() {
  try {
    const res = await getCourseDetail(courseId.value)
    detail.value = res.data
  } catch {
    /* 一次轮询失败不管：网络断了下一轮还会再试 */
  }
  syncPolling()
}

function startEdit() {
  form.name = detail.value.name
  form.intro = detail.value.intro || ''
  form.teacher = detail.value.teacher || ''
  editing.value = true
}

async function saveEdit() {
  try {
    await formRef.value.validate()
  } catch {
    return
  }
  saving.value = true
  try {
    await editCourse({ id: courseId.value, ...form })
    ElMessage.success('课程信息已保存')
    editing.value = false
    loadDetail()
  } catch (e) {
    ElMessage.error('保存失败：' + errText(e))
  } finally {
    saving.value = false
  }
}

// 点击视频 → 跳到观看页（路由由「课程观看」分支提供）
// 没有观看权限时（未登录 / 不在绑定班级）给出原因，而不是让学生点进去吃 403
function goWatch(video) {
  if (!detail.value?.canWatch) {
    ElMessage.warning(detail.value?.mediaReason || '登录后即可学习该课程')
    if (!localStorage.getItem('token')) {
      router.push({ path: '/login', query: { redirect: route.fullPath } })
    }
    return
  }
  router.push(`/course/${courseId.value}/watch/${video.id}`)
}

// 是否是课程创建者：只有创建者才能编辑 / 删除 / 上传。
// createdBy 为空（未知归属）时一律视为非创建者，只读。
const isOwner = computed(() => {
  if (!detail.value) return false
  const me = localStorage.getItem('username') || ''
  return !!me && me === detail.value.createdBy
})

// 删除课程：二次确认后调接口，成功跳回列表
async function onDelete() {
  try {
    await ElMessageBox.confirm(
      `确定要删除课程《${detail.value.name}》吗？课程下的视频 / 资源会一并清理。`,
      '删除课程',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  try {
    await deleteCourse(courseId.value)
    ElMessage.success('课程已删除')
    router.push('/courses')
  } catch (e) {
    ElMessage.error('删除失败：' + errText(e))
  }
}

function formatSize(bytes) {
  if (bytes == null) return '-'
  const units = ['B', 'KB', 'MB', 'GB']
  let i = 0
  let n = Number(bytes)
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024
    i++
  }
  return `${n.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

// 后端错误文案统一从响应体里取，取不到才退回异常本身
function errText(e) {
  return e?.response?.data?.msg || e?.message || '未知错误'
}

// 上传视频：小文件走表单直传，大文件（>250MB）走分片直传 + 断点续传。
//
// 分片路径的字节流不经过后端 API 容器，是浏览器 PUT 到预签名 URL，
// 所以这里的进度来自"已确认落地的分片"，比 axios 的已发字节可信。
const multipartApi = {
  initMultipartUpload,
  signMultipartParts,
  completeMultipartUpload,
  abortMultipartUpload,
  fetchMultipartUploadStatus,
}

// 阶段文案与百分比统一走 utils/uploadProgress.js：
// 同一个视频在创建页 / 详情页 / 列表快捷上传里传，进度必须长一个样。
const uploading = ref(false)
const uploadProgress = reactive(newProgressState())
const isMultipart = ref(false)
let uploadController = null

// 取消当前上传（分片路径才会用到；表单路径浏览器自己截断即可）
function cancelUpload() {
  uploadController?.abort()
}

async function onUploadVideo(option) {
  uploading.value = true
  resetProgressState(uploadProgress)
  uploadProgress.file = option.file?.name || ''
  uploadProgress.total = option.file?.size || 0
  isMultipart.value = shouldUseMultipart(option.file)
  uploadController = new AbortController()
  try {
    if (isMultipart.value) {
      await uploadByMultipart(option.file, {
        courseId: courseId.value,
        api: multipartApi,
        signal: uploadController.signal,
        // util 报 done 只代表分片合并完了，转码还在队列里排队
        onProgress: (p) =>
          applyProgressEvent(uploadProgress, { ...p, phase: p.phase === 'done' ? 'queued' : p.phase }),
      })
    } else {
      const fd = new FormData()
      fd.append('files', option.file)
      await uploadCourseVideos(
        courseId.value,
        fd,
        makeUploadProgressReporter(option.file.name, (p) => applyProgressEvent(uploadProgress, p)),
      )
      // 表单路径后端是一次性收完再落库，回包到这里才算真的传完
      applyProgressEvent(uploadProgress, { percent: DONE_PERCENT, phase: 'queued' })
    }
    ElMessage.success('视频上传成功，服务器正在排队转码')
    option.onSuccess?.(null)
    await loadDetail()
  } catch (e) {
    // 失败 / 取消时把进度条撤掉：停在 99%"服务端处理中"看着像还在传
    resetProgressState(uploadProgress)
    if (e?.name === 'UploadCancelled') {
      // 主动取消 = 服务端 abort + 清本地记账，分片已作废，重传是从头开始；
      // 能"重选同一文件续传"的是网络中断（util 保留了记账），不是取消。
      ElMessage.info('已取消上传：这次传的分片服务端已作废，重新上传该文件会从头开始')
    } else {
      ElMessage.error('上传失败：' + errText(e))
    }
    option.onError?.(e)
  } finally {
    uploading.value = false
    isMultipart.value = false
    uploadController = null
  }
}

// 转码失败后人工重新排队：只改本地这一行的状态，随后由轮询拿回权威结果
const retryingId = ref(0)
async function onRetryProcess(video) {
  retryingId.value = video.id
  try {
    await reprocessCourseVideo(courseId.value, video.id)
    video.processStatus = 'pending'
    video.processError = ''
    ElMessage.success('已重新排队转码')
    startPolling()
  } catch (e) {
    ElMessage.error('重新排队失败：' + errText(e))
  } finally {
    retryingId.value = 0
  }
}
</script>

<template>
  <div class="detail-page" v-loading="loading">
    <template v-if="detail">
      <!-- 顶部：封面 + 基本信息 -->
      <div class="hero">
        <div class="cover-box">
          <img v-if="detail.coverThumbUrl" :src="detail.coverThumbUrl" alt="课程封面" />
          <div v-else class="cover-fallback">无封面</div>
        </div>
        <div class="info">
          <template v-if="!editing">
            <h1 class="name">{{ detail.name }}</h1>
            <p class="teacher">授课教师：{{ detail.teacher || '未设置' }}</p>
            <p class="creator">创建者：{{ detail.createdBy || '系统预置' }}</p>
            <p class="intro">{{ detail.intro || '暂无简介' }}</p>
            <div v-if="isOwner" class="owner-actions">
              <el-button type="primary" plain @click="startEdit">编辑课程信息</el-button>
              <el-button type="danger" plain @click="onDelete">删除课程</el-button>
            </div>
            <span v-else class="viewer-tip">仅可浏览，编辑 / 上传需课程创建者操作</span>
          </template>

          <!-- 编辑模式 -->
          <el-form v-else ref="formRef" :model="form" :rules="rules" label-width="70px" class="edit-form">
            <el-form-item label="课程名称" prop="name">
              <el-input v-model="form.name" />
            </el-form-item>
            <el-form-item label="授课教师">
              <el-input v-model="form.teacher" placeholder="可选" />
            </el-form-item>
            <el-form-item label="简介">
              <el-input v-model="form.intro" type="textarea" :rows="3" placeholder="可选" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="saving" @click="saveEdit">保存</el-button>
              <el-button @click="editing = false">取消</el-button>
            </el-form-item>
          </el-form>
        </div>
      </div>

      <!-- 内容区：视频 / 资源 tab -->
      <el-tabs class="tabs" v-model="activeTab">
        <el-tab-pane label="课程视频" name="videos">
          <el-alert
            v-if="!detail.canWatch"
            class="lock-tip"
            type="info"
            :closable="false"
            show-icon
            :title="detail.mediaReason || '登录后即可学习该课程'"
          />
          <div v-if="isOwner" class="upload-bar">
            <el-upload
              :show-file-list="false"
              :http-request="onUploadVideo"
              accept="video/*"
              :disabled="uploading"
            >
              <el-button type="primary" :loading="uploading">
                {{ uploading ? '上传中...' : '上传视频' }}
              </el-button>
            </el-upload>
            <el-button v-if="uploading && isMultipart" type="danger" plain size="small" @click="cancelUpload">
              取消上传
            </el-button>
            <span class="upload-tip">
              {{
                isMultipart
                  ? '大文件分片直传中，中断后可重选同一文件续传'
                  : '支持 mp4/webm 等常见格式，单个最大 800MB；超过 250MB 自动改用分片直传'
              }}
            </span>
            <!-- 传完不收起：最后那条"排队转码中"正是异步后处理交接的提示 -->
            <div v-if="uploading || uploadProgress.phase" class="upload-progress">
              <el-progress :percentage="uploadProgress.percent" :stroke-width="14" />
              <span class="upload-phase">{{ progressLine(uploadProgress) }}</span>
            </div>
          </div>
          <div v-if="detail.videos && detail.videos.length" class="video-list">
            <div
              v-for="v in detail.videos"
              :key="v.id"
              class="video-item"
              @click="goWatch(v)"
            >
              <el-icon :size="28" color="#2563eb"><VideoPlay /></el-icon>
              <div class="video-meta">
                <span class="video-title">
                  <span class="video-name">{{ v.name }}</span>
                  <el-tooltip v-if="processStatusTag(v).hint" :content="processStatusTag(v).hint" placement="top">
                    <el-tag
                      size="small"
                      :type="processStatusTag(v).type"
                      :effect="processStatusTag(v).status === 'done' ? 'plain' : 'light'"
                    >{{ processStatusTag(v).text }}</el-tag>
                  </el-tooltip>
                  <el-tag
                    v-else
                    size="small"
                    :type="processStatusTag(v).type"
                    :effect="processStatusTag(v).status === 'done' ? 'plain' : 'light'"
                  >{{ processStatusTag(v).text }}</el-tag>
                </span>
                <span class="video-sub">{{ formatSize(v.size) }} · {{ v.uploadedAt }}</span>
                <span v-if="processStatusTag(v).status === 'failed'" class="video-hint">
                  {{ processStatusTag(v).hint }}
                </span>
              </div>
              <el-button
                v-if="isOwner && canRetryProcess(v)"
                size="small"
                type="warning"
                plain
                :loading="retryingId === v.id"
                @click.stop="onRetryProcess(v)"
              >重试转码</el-button>
              <el-button size="small" type="primary" link>播放</el-button>
            </div>
          </div>
          <el-empty v-else description="还没有视频，去「创建课程」上传视频吧" />
        </el-tab-pane>

        <el-tab-pane label="课程资源" name="resources">
          <CourseResourcePanel
            v-if="detail.canWatch"
            :course-id="courseId"
            :can-edit="isOwner"
          />
          <el-alert
            v-else
            type="info"
            :closable="false"
            show-icon
            :title="detail.mediaReason || '登录后即可查看课程资料'"
          />
        </el-tab-pane>
      </el-tabs>
    </template>
  </div>
</template>

<style scoped>
.detail-page {
  max-width: 1000px;
  margin: 0 auto;
  padding: 24px 16px 48px;
  min-height: 60vh;
}

.hero {
  display: flex;
  gap: 24px;
  background: #fff;
  border-radius: 16px;
  padding: 22px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
}

.cover-box {
  width: 260px;
  height: 150px;
  border-radius: 10px;
  overflow: hidden;
  flex-shrink: 0;
  background: #f3f4f6;
}

.cover-box img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.cover-fallback {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #9ca3af;
  font-size: 14px;
}

.info {
  flex: 1;
  min-width: 0;
}

.name {
  margin: 0 0 8px;
  font-size: 24px;
  color: #1f2937;
}

.teacher {
  color: #6b7280;
  font-size: 14px;
  margin: 0 0 4px;
}

.creator {
  color: #9ca3af;
  font-size: 13px;
  margin: 0 0 10px;
}

.intro {
  color: #4b5563;
  font-size: 14px;
  line-height: 1.7;
  margin: 0 0 14px;
}

.owner-actions {
  display: flex;
  gap: 10px;
}

.viewer-tip {
  font-size: 13px;
  color: #9ca3af;
}

.tabs {
  margin-top: 24px;
  background: #fff;
  border-radius: 12px;
  padding: 8px 18px 18px;
}

.video-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.upload-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  margin: 12px 0 16px;
}

.upload-progress {
  flex-basis: 100%;
  display: flex;
  align-items: center;
  gap: 12px;
}

.upload-progress :deep(.el-progress) {
  flex: 1;
}

.upload-phase {
  font-size: 12px;
  color: #2563eb;
  white-space: nowrap;
}

.lock-tip {
  margin: 12px 0;
}

.upload-tip {
  font-size: 12px;
  color: #9ca3af;
}

.video-item {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 12px 14px;
  border: 1px solid #eef0f3;
  border-radius: 10px;
  cursor: pointer;
  transition: background 0.15s;
}

.video-item:hover {
  background: #f8fafc;
}

.video-meta {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.video-name {
  font-size: 14px;
  color: #1f2937;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.video-title {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.video-title .video-name {
  flex: 0 1 auto;
}

.video-title :deep(.el-tag) {
  flex-shrink: 0;
}

/* 转码说明只留一行，超长省略；完整文案交给 tooltip */
.video-hint {
  font-size: 12px;
  color: #b45309;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.video-sub {
  font-size: 12px;
  color: #9ca3af;
}
</style>

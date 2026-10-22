<script setup>
import { ref, reactive, onMounted, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { VideoPlay } from '@element-plus/icons-vue'
import { getCourseDetail, editCourse } from '../api/index.js'

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
  } catch (e) {
    ElMessage.error('课程详情加载失败：' + (e.response?.data?.msg || e.message))
  } finally {
    loading.value = false
  }
}

onMounted(loadDetail)

// 路由参数变化（同组件内切换课程）时重新加载
watch(courseId, loadDetail)

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
    ElMessage.error('保存失败：' + (e.response?.data?.msg || e.message))
  } finally {
    saving.value = false
  }
}

// 点击视频 → 跳到观看页（路由由「课程观看」分支提供）
function goWatch(video) {
  router.push(`/course/${courseId.value}/watch/${video.id}`)
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
            <p class="intro">{{ detail.intro || '暂无简介' }}</p>
            <el-button type="primary" plain @click="startEdit">编辑课程信息</el-button>
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
          <div v-if="detail.videos && detail.videos.length" class="video-list">
            <div
              v-for="v in detail.videos"
              :key="v.id"
              class="video-item"
              @click="goWatch(v)"
            >
              <el-icon :size="28" color="#2563eb"><VideoPlay /></el-icon>
              <div class="video-meta">
                <span class="video-name">{{ v.name }}</span>
                <span class="video-sub">{{ formatSize(v.size) }} · {{ v.uploadedAt }}</span>
              </div>
              <el-button size="small" type="primary" link>播放</el-button>
            </div>
          </div>
          <el-empty v-else description="还没有视频，去「创建课程」上传视频吧" />
        </el-tab-pane>

        <el-tab-pane label="课程资源" name="resources">
          <!-- 课程资源分支：此处替换为 <CourseResourcePanel :course-id="courseId" /> -->
          <el-empty description="课程讲义 / PPT / 文档资源即将上线" />
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
  margin: 0 0 10px;
}

.intro {
  color: #4b5563;
  font-size: 14px;
  line-height: 1.7;
  margin: 0 0 14px;
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

.video-sub {
  font-size: 12px;
  color: #9ca3af;
}
</style>

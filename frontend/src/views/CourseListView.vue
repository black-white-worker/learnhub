<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { fetchCourseList } from '../api/index.js'

const router = useRouter()

/* 课程列表数据（来自后端 GET /api/course/list） */
const courses = ref([])
const loading = ref(false)

async function loadCourses() {
  loading.value = true
  try {
    const res = await fetchCourseList()
    courses.value = res.data.courses || []
  } catch (e) {
    ElMessage.error('课程列表加载失败：' + (e.response?.data?.msg || e.message))
  } finally {
    loading.value = false
  }
}

onMounted(loadCourses)

// 点击卡片 → 进入课程详情页（编辑/删除都在详情页里操作）
function goDetail(course) {
  router.push(`/course/${course.id}`)
}

// 封面缺省时的兜底底色（按课程名 hash 取一个柔和色）
function coverBg(course) {
  const palette = [
    'linear-gradient(135deg,#2563eb,#60a5fa)',
    'linear-gradient(135deg,#7c3aed,#a78bfa)',
    'linear-gradient(135deg,#059669,#34d399)',
    'linear-gradient(135deg,#ea580c,#fb923c)',
    'linear-gradient(135deg,#db2777,#f472b6)',
  ]
  let sum = 0
  for (const ch of course.name) sum += ch.charCodeAt(0)
  return palette[sum % palette.length]
}
</script>

<template>
  <div class="course-list-page" v-loading="loading">
    <div class="page-header">
      <h2>我的课程</h2>
      <div class="header-actions">
        <el-button @click="loadCourses">刷新</el-button>
        <el-button type="primary" @click="router.push('/course/create')">新建课程</el-button>
      </div>
    </div>

    <div v-if="courses.length" class="course-grid">
      <div
        v-for="c in courses"
        :key="c.id"
        class="course-card"
        @click="goDetail(c)"
      >
        <!-- 封面区：有缩略图用缩略图，否则用渐变色 + 课程名首字 -->
        <div class="cover" :style="c.coverThumbUrl ? {} : { background: coverBg(c) }">
          <img v-if="c.coverThumbUrl" :src="c.coverThumbUrl" :alt="c.name" />
          <span v-else class="cover-placeholder">{{ c.name.slice(0, 1) }}</span>
        </div>
        <div class="body">
          <h3 class="title">{{ c.name }}</h3>
          <p class="meta">{{ c.teacher || '未设教师' }} · {{ (c.videos || []).length }} 个视频</p>
        </div>
      </div>
    </div>

    <el-empty v-else-if="!loading" description="暂无课程，点击右上角「新建课程」开始" />
  </div>
</template>

<style scoped>
.course-list-page {
  max-width: 1100px;
  margin: 0 auto;
  padding: 24px 16px 40px;
  min-height: 60vh;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
}

.page-header h2 {
  margin: 0;
}

.header-actions {
  display: flex;
  gap: 10px;
}

.course-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 18px;
}

.course-card {
  background: #fff;
  border-radius: 14px;
  overflow: hidden;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
  cursor: pointer;
  transition: transform 0.2s, box-shadow 0.2s;
}

.course-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 12px 28px rgba(37, 99, 235, 0.16);
}

.cover {
  position: relative;
  height: 140px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.cover-placeholder {
  font-size: 44px;
  color: rgba(255, 255, 255, 0.92);
  font-weight: 700;
}

.body {
  padding: 12px 14px 14px;
}

.title {
  font-size: 15px;
  margin: 0 0 6px;
  color: #1f2937;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.meta {
  font-size: 12px;
  color: #9ca3af;
  margin: 0;
}
</style>

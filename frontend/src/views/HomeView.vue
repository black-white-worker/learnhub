<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { fetchCourseList } from '../api/index.js'

const router = useRouter()

/* ===== 真实课程数据（来自后端 GET /api/course/list），替换原静态 mock ===== */
const courses = ref([])
const loading = ref(false)
const keyword = ref('')

async function loadCourses() {
  loading.value = true
  try {
    const res = await fetchCourseList()
    courses.value = res.data.courses || []
  } catch {
    courses.value = []
  } finally {
    loading.value = false
  }
}

onMounted(loadCourses)

// 搜索框真实过滤课程名 / 教师（大小写不敏感）
const filteredCourses = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return courses.value
  return courses.value.filter(
    (c) =>
      (c.name || '').toLowerCase().includes(kw) ||
      (c.teacher || '').toLowerCase().includes(kw),
  )
})

function goDetail(course) {
  router.push(`/course/${course.id}`)
}

// 封面缺省时的兜底色
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
  <div class="home">
    <!-- 1. 首屏 Hero 横幅 -->
    <section class="hero">
      <div class="hero-inner">
        <h1 class="hero-title">你好，同学 👋</h1>
        <p class="hero-sub">让每一门课都学有所得，让每一次进步都被看见</p>
        <div class="hero-search">
          <el-input
            v-model="keyword"
            placeholder="搜索课程 / 老师"
            size="large"
            clearable
          />
        </div>
      </div>
    </section>

    <!-- 2. 课程区：真实课程卡片，点击进入详情 -->
    <section class="section" v-loading="loading">
      <div class="section-header">
        <h2>我的课程</h2>
        <span class="count">共 {{ filteredCourses.length }} 门</span>
      </div>

      <div v-if="filteredCourses.length" class="course-grid">
        <div
          v-for="c in filteredCourses"
          :key="c.id"
          class="course-card"
          @click="goDetail(c)"
        >
          <div class="course-cover" :style="c.coverThumbUrl ? {} : { background: coverBg(c) }">
            <img v-if="c.coverThumbUrl" :src="c.coverThumbUrl" :alt="c.name" />
            <span v-else class="course-emoji">{{ c.name.slice(0, 1) }}</span>
          </div>
          <div class="course-body">
            <h3 class="course-title">{{ c.name }}</h3>
            <p class="course-meta">{{ c.teacher || '未设教师' }} · {{ (c.videos || []).length }} 个视频</p>
          </div>
        </div>
      </div>
      <el-empty v-else description="还没有课程，去「课程中心 - 创建课程」开始吧" />
    </section>
  </div>
</template>

<style scoped>
.home {
  max-width: 1200px;
  margin: 0 auto;
  padding: 0 16px 32px;
}

/* ===== Hero 横幅 ===== */
.hero {
  border-radius: 0 0 24px 24px;
  background: linear-gradient(135deg, #1e3a8a 0%, #2563eb 55%, #3b82f6 100%);
  color: #fff;
  padding: 48px 24px 40px;
  text-align: center;
}

.hero-title {
  font-size: 32px;
  margin: 0 0 10px;
  font-weight: 700;
}

.hero-sub {
  font-size: 16px;
  opacity: 0.9;
  margin: 0 0 26px;
}

.hero-search {
  max-width: 520px;
  margin: 0 auto;
}

/* ===== 课程区块 ===== */
.section {
  margin-top: 28px;
}

.section-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin-bottom: 16px;
}

.section-header h2 {
  font-size: 20px;
  margin: 0;
}

.count {
  color: #9ca3af;
  font-size: 13px;
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
  box-shadow: 0 10px 24px rgba(37, 99, 235, 0.15);
}

.course-cover {
  height: 140px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.course-cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.course-emoji {
  font-size: 44px;
  color: rgba(255, 255, 255, 0.9);
  font-weight: 700;
}

.course-body {
  padding: 12px 14px 14px;
}

.course-title {
  font-size: 15px;
  margin: 0 0 6px;
  color: #1f2937;
}

.course-meta {
  font-size: 12px;
  color: #9ca3af;
  margin: 0;
}

/* ===== 响应式 ===== */
@media (max-width: 900px) {
  .course-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>

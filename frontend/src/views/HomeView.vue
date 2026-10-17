<script setup>
import { ref, computed } from 'vue'
import { ElMessage } from 'element-plus'
import {
  Reading, EditPen, Notebook, Folder, Medal, Bell, Calendar, DataAnalysis,
} from '@element-plus/icons-vue'
import request from '../api/index.js'

/* ===== mock 课程数据（后续替换为后端接口） ===== */
const courses = ref([
  { id: 1, title: 'Go 语言从入门到实战', teacher: '张老师', students: 3280, progress: 72, category: '编程开发', color: 'linear-gradient(135deg,#2563eb,#60a5fa)' },
  { id: 2, title: 'Vue3 前端开发实战', teacher: '李老师', students: 2650, progress: 45, category: '编程开发', color: 'linear-gradient(135deg,#7c3aed,#a78bfa)' },
  { id: 3, title: '高等数学（一）', teacher: '王教授', students: 5120, progress: 30, category: '数学基础', color: 'linear-gradient(135deg,#059669,#34d399)' },
  { id: 4, title: '大学英语四级通关', teacher: '陈老师', students: 8900, progress: 88, category: '外语学习', color: 'linear-gradient(135deg,#ea580c,#fb923c)' },
  { id: 5, title: 'UI 设计基础入门', teacher: '赵老师', students: 1930, progress: 15, category: '设计创意', color: 'linear-gradient(135deg,#db2777,#f472b6)' },
  { id: 6, title: '计算机组成原理', teacher: '刘教授', students: 2210, progress: 60, category: '编程开发', color: 'linear-gradient(135deg,#0891b2,#22d3ee)' },
])

/* ===== 课程分类筛选（腾讯课堂式横向分类条） ===== */
const categories = ['全部', '编程开发', '外语学习', '数学基础', '设计创意', '考试认证']
const activeCategory = ref('全部')

const filteredCourses = computed(() =>
  activeCategory.value === '全部'
    ? courses.value
    : courses.value.filter((c) => c.category === activeCategory.value)
)

/* ===== 快捷功能宫格（学习通式） ===== */
const quickActions = [
  { label: '我的课程', icon: Notebook },
  { label: '在线考试', icon: EditPen },
  { label: '课后作业', icon: Reading },
  { label: '学习资料', icon: Folder },
  { label: '成绩单', icon: Medal },
  { label: '消息通知', icon: Bell },
  { label: '课程签到', icon: Calendar },
  { label: '学习分析', icon: DataAnalysis },
]

function onQuickAction(label) {
  ElMessage.info('「' + label + '」建设中，敬请期待')
}

function goCourseDetail(title) {
  ElMessage.info('《' + title + '》课程详情建设中，敬请期待')
}

/* ===== 平台公告 ===== */
const notices = [
  { title: '2026 秋季学期选课开始啦', date: '10-15' },
  { title: '平台将于本周日 00:00-02:00 升级维护', date: '10-14' },
  { title: '新增 AI 智能答疑功能（内测中）', date: '10-12' },
  { title: '《Go 语言实战》课程作业截止提醒', date: '10-10' },
]

/* ===== 后端连通测试 ===== */
const keyword = ref('')
const result = ref('')
const loading = ref(false)

async function testBackend() {
  loading.value = true
  result.value = ''
  try {
    const res = await request.get('/from/me')
    result.value = '后端连接正常：' + JSON.stringify(res.data)
  } catch (e) {
    result.value = '请求失败：' + (e.response?.data?.msg || e.message)
  } finally {
    loading.value = false
  }
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
            placeholder="搜索课程 / 老师 / 知识点"
            size="large"
            clearable
          >
            <template #append>
              <el-button type="primary" @click="ElMessage.info('搜索功能建设中')">搜索</el-button>
            </template>
          </el-input>
        </div>
        <div class="hero-stats">
          <div class="stat"><b>128</b><span>优质课程</span></div>
          <div class="stat"><b>5.2万</b><span>注册学员</span></div>
          <div class="stat"><b>96%</b><span>课程完成率</span></div>
        </div>
      </div>
    </section>

    <!-- 2. 快捷功能宫格 -->
    <section class="section">
      <div class="quick-grid">
        <div
          v-for="a in quickActions"
          :key="a.label"
          class="quick-card"
          @click="onQuickAction(a.label)"
        >
          <el-icon :size="26" color="var(--el-color-primary)">
            <component :is="a.icon" />
          </el-icon>
          <span>{{ a.label }}</span>
        </div>
      </div>
    </section>

    <!-- 3. 课程分类 + 课程卡片网格 -->
    <section class="section">
      <div class="section-header">
        <h2>热门课程</h2>
        <div class="category-tabs">
          <span
            v-for="c in categories"
            :key="c"
            class="cat-tab"
            :class="{ active: c === activeCategory }"
            @click="activeCategory = c"
          >{{ c }}</span>
        </div>
      </div>

      <div class="course-grid">
        <div v-for="c in filteredCourses" :key="c.id" class="course-card">
          <div class="course-cover" :style="{ background: c.color }">
            <el-tag size="small" effect="dark" class="course-tag">{{ c.category }}</el-tag>
            <span class="course-emoji">{{ c.title.slice(0, 1) }}</span>
          </div>
          <div class="course-body">
            <h3 class="course-title">{{ c.title }}</h3>
            <p class="course-meta">{{ c.teacher }} · {{ c.students }} 人在学</p>
            <el-progress :percentage="c.progress" :stroke-width="8" :show-text="false" />
            <div class="course-footer">
              <span class="progress-text">已学 {{ c.progress }}%</span>
              <el-button type="primary" size="small" @click="goCourseDetail(c.title)">继续学习</el-button>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 4. 公告 + 后端测试 -->
    <section class="section bottom-section">
      <el-card shadow="never" class="panel">
        <template #header>平台公告</template>
        <ul class="notice-list">
          <li v-for="n in notices" :key="n.title">
            <span class="notice-title">{{ n.title }}</span>
            <span class="notice-date">{{ n.date }}</span>
          </li>
        </ul>
      </el-card>

      <el-card shadow="never" class="panel">
        <template #header>开发调试</template>
        <el-button type="primary" :loading="loading" @click="testBackend">
          测试后端连接
        </el-button>
        <p v-if="result" class="result">{{ result }}</p>
      </el-card>
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
  padding: 56px 24px 48px;
  text-align: center;
}

.hero-title {
  font-size: 34px;
  margin: 0 0 12px;
  font-weight: 700;
}

.hero-sub {
  font-size: 16px;
  opacity: 0.9;
  margin: 0 0 28px;
}

.hero-search {
  max-width: 520px;
  margin: 0 auto 32px;
}

.hero-search :deep(.el-input-group__append) {
  background: var(--el-color-primary);
  border-color: var(--el-color-primary);
}

.hero-stats {
  display: flex;
  justify-content: center;
  gap: 56px;
}

.stat {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.stat b {
  font-size: 26px;
  font-weight: 700;
}

.stat span {
  font-size: 13px;
  opacity: 0.85;
}

/* ===== 通用区块 ===== */
.section {
  margin-top: 28px;
}

/* ===== 快捷宫格 ===== */
.quick-grid {
  display: grid;
  grid-template-columns: repeat(8, 1fr);
  gap: 14px;
}

.quick-card {
  background: #fff;
  border-radius: 12px;
  padding: 20px 8px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  box-shadow: var(--app-card-shadow);
  transition: transform 0.2s, box-shadow 0.2s;
  font-size: 13px;
  color: #374151;
}

.quick-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 8px 20px rgba(37, 99, 235, 0.15);
}

/* ===== 课程区块 ===== */
.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.section-header h2 {
  font-size: 20px;
  margin: 0;
}

.category-tabs {
  display: flex;
  gap: 8px;
}

.cat-tab {
  padding: 6px 14px;
  border-radius: 999px;
  font-size: 13px;
  cursor: pointer;
  color: #4b5563;
  background: #fff;
  border: 1px solid #e5e7eb;
  transition: all 0.2s;
}

.cat-tab:hover {
  color: var(--el-color-primary);
  border-color: var(--el-color-primary);
}

.cat-tab.active {
  background: var(--el-color-primary);
  color: #fff;
  border-color: var(--el-color-primary);
  font-weight: 600;
}

.course-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 18px;
}

.course-card {
  background: #fff;
  border-radius: 14px;
  overflow: hidden;
  box-shadow: var(--app-card-shadow);
  transition: transform 0.2s, box-shadow 0.2s;
  cursor: pointer;
}

.course-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 10px 24px rgba(37, 99, 235, 0.15);
}

.course-cover {
  height: 130px;
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
}

.course-tag {
  position: absolute;
  top: 10px;
  left: 10px;
}

.course-emoji {
  font-size: 44px;
  color: rgba(255, 255, 255, 0.9);
  font-weight: 700;
}

.course-body {
  padding: 14px 16px 16px;
}

.course-title {
  font-size: 15px;
  margin: 0 0 6px;
  color: #1f2937;
}

.course-meta {
  font-size: 12px;
  color: #9ca3af;
  margin: 0 0 12px;
}

.course-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 10px;
}

.progress-text {
  font-size: 12px;
  color: #6b7280;
}

/* ===== 底部面板 ===== */
.bottom-section {
  display: grid;
  grid-template-columns: 2fr 1fr;
  gap: 18px;
}

.panel :deep(.el-card__header) {
  font-weight: 600;
  font-size: 15px;
}

.notice-list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.notice-list li {
  display: flex;
  justify-content: space-between;
  padding: 10px 0;
  border-bottom: 1px dashed #e5e7eb;
  font-size: 14px;
  color: #374151;
  cursor: pointer;
}

.notice-list li:hover {
  color: var(--el-color-primary);
}

.notice-list li:last-child {
  border-bottom: none;
}

.notice-date {
  color: #9ca3af;
  font-size: 13px;
}

.result {
  margin-top: 12px;
  font-size: 13px;
  color: var(--el-color-primary);
}

/* ===== 响应式 ===== */
@media (max-width: 900px) {
  .quick-grid {
    grid-template-columns: repeat(4, 1fr);
  }

  .course-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .bottom-section {
    grid-template-columns: 1fr;
  }
}
</style>

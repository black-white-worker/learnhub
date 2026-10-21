<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { fetchCourseList } from '../api/index.js'

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
</script>

<template>
  <div class="course-list-page">
    <div class="page-header">
      <h2>课程列表</h2>
      <el-button type="primary" :loading="loading" @click="loadCourses">刷新</el-button>
    </div>

    <el-table
      v-loading="loading"
      :data="courses"
      border
      stripe
      empty-text="暂无课程，请先创建课程"
    >
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column prop="name" label="课程名称" min-width="180" />
      <el-table-column label="封面" width="140">
        <!-- 有缩略图就展示封面缩略图（MinIO 上传后才有），否则显示占位 -->
        <template #default="{ row }">
          <img v-if="row.coverThumbUrl" :src="row.coverThumbUrl" alt="封面" class="cover-thumb" />
          <span v-else class="no-cover">无封面</span>
        </template>
      </el-table-column>
      <el-table-column prop="intro" label="课程简介" min-width="280" show-overflow-tooltip />
      <el-table-column prop="teacher" label="授课教师" width="140" />
      <el-table-column label="视频数" width="90">
        <template #default="{ row }">{{ (row.videos || []).length }}</template>
      </el-table-column>
    </el-table>
  </div>
</template>

<style scoped>
.course-list-page {
  max-width: 1000px;
  margin: 0 auto;
  padding: 24px 16px;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.page-header h2 {
  margin: 0;
}

.cover-thumb {
  width: 96px;
  height: 54px;
  object-fit: cover;
  border-radius: 6px;
  display: block;
}

.no-cover {
  color: #bbb;
  font-size: 13px;
}
</style>

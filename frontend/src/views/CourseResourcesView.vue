<script setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import CourseResourcePanel from '../components/CourseResourcePanel.vue'

const route = useRoute()
const router = useRouter()
const courseId = computed(() => Number(route.params.id))
</script>

<template>
  <div class="resources-page">
    <div class="page-head">
      <el-button link @click="router.back()">← 返回</el-button>
      <h2>课程资源</h2>
    </div>
    <CourseResourcePanel :course-id="courseId" />
  </div>
</template>

<style scoped>
.resources-page {
  max-width: 900px;
  margin: 0 auto;
  padding: 24px 16px 48px;
  min-height: 60vh;
}

.page-head {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 18px;
}

.page-head h2 {
  margin: 0;
}
</style>

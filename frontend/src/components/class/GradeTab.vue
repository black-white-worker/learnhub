<template>
  <el-table :data="gradeRows" size="small" style="margin-top:10px">
    <el-table-column prop="username" label="学生" />
    <el-table-column prop="avgScore" label="作业平均分" />
    <el-table-column prop="assignmentCnt" label="已交作业" />
    <el-table-column prop="signinCnt" label="签到次数" />
    <el-table-column prop="totalScore" label="加权总分">
      <template #default="{ row }">{{ row.totalScore.toFixed(1) }}</template>
    </el-table-column>
  </el-table>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import axios from 'axios'

defineProps({
  classId: { type: [String, Number], required: true },
})

const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }

const gradeRows = ref([])

async function loadGrades() {
  try {
    const r = await axios.get(`/api/class/${props.classId}/grades`, auth)
    gradeRows.value = r.data?.rows || []
  } catch (e) {
    gradeRows.value = []
  }
}

onMounted(loadGrades)
</script>

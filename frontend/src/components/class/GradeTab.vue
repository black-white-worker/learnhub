<template>
  <div class="grade-tab">
    <div class="toolbar">
      <div class="rule">
        权重：视频 {{ rule.video }}% · 作业 {{ rule.assignment }}% · 签到 {{ rule.signin }}% · 考试 {{ rule.exam }}%
        <span class="hint">（空分类不占权重，每项按真实得分率折算）</span>
      </div>
      <el-button size="small" type="primary" :loading="exporting" @click="exportCsv">
        导出成绩单
      </el-button>
    </div>

    <el-table :data="gradeRows" size="small" style="margin-top:10px" empty-text="还没有成绩数据">
      <el-table-column prop="username" label="学生" min-width="120" />
      <el-table-column label="作业" min-width="110">
        <template #default="{ row }">
          {{ row.assignmentPercent.toFixed(1) }}%
          <span class="sub">({{ row.assignmentCnt }} 份)</span>
        </template>
      </el-table-column>
      <el-table-column label="考试" min-width="90">
        <template #default="{ row }">{{ row.examPercent.toFixed(1) }}%</template>
      </el-table-column>
      <el-table-column label="签到" min-width="100">
        <template #default="{ row }">
          {{ row.signinCnt }}/{{ row.signinTotal }}
          <span class="sub">({{ row.signinPercent.toFixed(0) }}%)</span>
        </template>
      </el-table-column>
      <el-table-column label="视频" min-width="100">
        <template #default="{ row }">
          {{ row.videoDone }}/{{ row.videoTotal }}
          <span class="sub">({{ row.videoPercent.toFixed(0) }}%)</span>
        </template>
      </el-table-column>
      <el-table-column label="加权总分" min-width="110">
        <template #default="{ row }">
          <strong>{{ row.totalScore.toFixed(1) }}</strong>
          <el-tag size="small" :type="tagType(row.letter)" style="margin-left:6px">{{ row.letter }}</el-tag>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup>
// 成绩册（对标 Canvas gradebook）。
//
// 改造前只有"作业平均分 / 已交作业 / 签到次数 / 加权总分"四列，
// 且加权总分是后端算错的（视频项写死满分、签到不封顶）。
// 现在展示的是四类真实得分率 + 加权总分 + 等级，支持一键导出 CSV。
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import axios from 'axios'
import { downloadFile, toastError } from '../../api/client.js'

const props = defineProps({
  classId: { type: [String, Number], required: true },
})

const gradeRows = ref([])
const rule = ref({ video: 0, assignment: 0, signin: 0, exam: 0 })
const exporting = ref(false)

async function loadGrades() {
  try {
    const r = await axios.get(`/api/class/${props.classId}/grades`)
    gradeRows.value = r.data?.rows || []
    if (r.data?.rule) rule.value = r.data.rule
  } catch (e) {
    gradeRows.value = []
    toastError(e, '加载成绩册失败')
  }
}

async function exportCsv() {
  exporting.value = true
  try {
    await downloadFile(`/api/class/${props.classId}/grades/export`, '成绩单.csv')
    ElMessage.success('成绩单已开始下载')
  } catch (e) {
    toastError(e, '导出失败')
  } finally {
    exporting.value = false
  }
}

function tagType(letter) {
  if (letter === 'A') return 'success'
  if (letter === 'B') return 'primary'
  if (letter === 'C') return 'warning'
  if (letter === 'D') return 'info'
  return 'danger'
}

onMounted(loadGrades)
</script>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.rule {
  font-size: 13px;
  color: #475569;
}
.hint {
  color: #94a3b8;
  margin-left: 4px;
}
.sub {
  color: #94a3b8;
  font-size: 12px;
}
</style>

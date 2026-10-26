<script setup>
// 作业详情页：老师端用 QuestionEditor 出题器；学生端问卷式答题+文件上传。
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import axios from 'axios'
import QuestionEditor from '../components/QuestionEditor.vue'

const route = useRoute()
const router = useRouter()
const aid = route.params.id
const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }

const detail = ref(null)
const answers = ref({})
const submitting = ref(false)
const myFileUrl = ref('')
const uploading = ref(false)

const uploadAction = '/api/assignment/upload'
const uploadHeaders = { Authorization: 'Bearer ' + token }

const isTeacher = computed(() => detail.value?.myRole === 'teacher' || detail.value?.myRole === 'admin')

async function load() {
  try {
    const r = await axios.get(`/api/assignment/${aid}`, auth)
    detail.value = r.data
    if (r.data.mySubmission?.answers) {
      try { answers.value = JSON.parse(r.data.mySubmission.answers) } catch (e) { answers.value = {} }
    }
    myFileUrl.value = r.data.mySubmission?.fileUrl || ''
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '加载作业失败')
  }
}

function beforeUpload(file) {
  if (file.size > 50 * 1024 * 1024) {
    ElMessage.error('附件不能超过 50MB'); return false
  }
  uploading.value = true
  return true
}
function onUploadSuccess(res) {
  uploading.value = false
  myFileUrl.value = res.url
  ElMessage.success('附件上传成功')
}
function onUploadError() {
  uploading.value = false
  ElMessage.error('上传失败')
}

async function submit() {
  try {
    await ElMessageBox.confirm('确定提交作业吗？', '提示', { type: 'warning' })
  } catch { return }
  submitting.value = true
  try {
    await axios.post(`/api/assignment/${aid}/submit`, {
      content: answers.value['essay-content'] || '',
      fileUrl: myFileUrl.value,
      answers: answers.value,
    }, auth)
    ElMessage.success('提交成功')
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '提交失败')
  } finally {
    submitting.value = false
  }
}

onMounted(load)
</script>

<template>
  <div v-if="detail">
    <!-- 老师端：出题器 -->
    <template v-if="isTeacher">
      <div style="display:flex;align-items:center;gap:12px;padding:12px 16px;background:#fff;border-bottom:1px solid #eee">
        <el-button @click="$router.back()" size="small">← 返回</el-button>
        <h2 style="margin:0">{{ detail.assignment.title }}</h2>
      </div>
      <QuestionEditor
        module="assignment"
        :resource-id="aid"
        :initial-questions="detail.questions"
        :initial-settings="{
          title: detail.assignment.title,
          description: detail.assignment.description,
          deadline: (detail.assignment.deadline || '').replace(' ', 'T'),
          lateDeadline: (detail.assignment.lateDeadline || '').replace(' ', 'T'),
          totalScore: detail.assignment.totalScore,
          allowLate: detail.assignment.allowLate,
        }"
        :available-types="['single','multiple','fill','essay','fileupload']"
      />

      <el-divider />
      <h3 style="margin-left:12px">学生提交（{{ detail.submissions?.length || 0 }} 人）</h3>
      <el-table :data="detail.submissions || []" size="small" style="margin:0 12px" max-height="400">
        <el-table-column prop="username" label="学生" width="120" />
        <el-table-column prop="autoScore" label="客观题分" width="100" />
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag v-if="row.score !== null && row.score !== undefined" type="success" size="small">已批改</el-tag>
            <el-tag v-else type="warning" size="small">待批改</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="附件" width="100">
          <template #default="{ row }">
            <a v-if="row.fileUrl" :href="row.fileUrl" target="_blank">查看</a>
            <span v-else>-</span>
          </template>
        </el-table-column>
      </el-table>
    </template>

    <!-- 学生端 -->
    <template v-else>
      <div style="max-width:800px;margin:24px auto;padding:0 16px">
        <el-button @click="$router.back()" size="small">← 返回</el-button>
        <h2>{{ detail.assignment.title }}</h2>
        <p style="color:#666">{{ detail.assignment.description }}</p>
        <el-alert v-if="detail.mySubmission" type="success" :closable="false"
          :title="`已提交，得分：${detail.mySubmission.score ?? detail.mySubmission.autoScore ?? '待批改'}`" />

        <div v-for="(q, i) in detail.questions" :key="q.id" class="q-card">
          <div class="q-head">
            <span class="q-no">{{ i + 1 }}. {{ q.stem }}</span>
            <span class="q-score">{{ q.score }}分</span>
          </div>
          <el-radio-group v-if="q.type === 'single'" v-model="answers[q.id]" style="display:flex;flex-direction:column;gap:8px">
            <el-radio v-for="(opt, oi) in (q.options ? JSON.parse(q.options) : [])" :key="oi" :value="'ABCD'[oi]">
              {{ 'ABCD'[oi] }}. {{ opt }}
            </el-radio>
          </el-radio-group>
          <el-checkbox-group v-else-if="q.type === 'multiple'"
            :model-value="answers[q.id] ? answers[q.id].split(',').filter(Boolean) : []"
            @change="val => answers[q.id] = [...val].sort().join(',')"
            style="display:flex;flex-direction:column;gap:8px">
            <el-checkbox v-for="(opt, oi) in (q.options ? JSON.parse(q.options) : [])" :key="oi" :value="'ABCD'[oi]">
              {{ 'ABCD'[oi] }}. {{ opt }}
            </el-checkbox>
          </el-checkbox-group>
          <el-input v-else-if="q.type === 'fileupload'" v-model="answers[q.id]" placeholder="上传文件链接或说明">
            <template #append>
              <el-upload :action="uploadAction" :headers="uploadHeaders" :show-file-list="false"
                :before-upload="beforeUpload" :on-success="onUploadSuccess" :on-error="onUploadError">
                <el-button size="small">选择文件</el-button>
              </el-upload>
            </template>
          </el-input>
          <el-input v-else v-model="answers[q.id]" type="textarea" :rows="5" placeholder="请输入作答" />
          <div v-if="myFileUrl && q.type === 'fileupload'" style="margin-top:8px">
            已传：<a :href="myFileUrl" target="_blank">{{ myFileUrl }}</a>
          </div>
        </div>

        <el-button v-if="!detail.mySubmission" type="primary" size="large" :loading="submitting" @click="submit" style="margin-top:16px">
          提交作业
        </el-button>
      </div>
    </template>
  </div>
</template>

<style scoped>
.q-card { background:#fff;border-radius:10px;padding:20px;margin-bottom:14px;box-shadow:0 2px 8px rgba(0,0,0,.05); }
.q-head { display:flex;justify-content:space-between;align-items:center;margin-bottom:12px; }
.q-no { font-size:16px;font-weight:600; }
.q-score { color:#999;font-size:13px; }
</style>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import axios from 'axios'

const router = useRouter()
const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }

const classes = ref([])
const examsByClass = ref({}) // { classId: [exam] }
const loading = ref(false)

const showCreate = ref(false)
const form = ref({
  classId: null,
  title: '',
  description: '',
  startAt: '',
  endAt: '',
  durationMinutes: 60,
  totalScore: 100,
})

async function load() {
  loading.value = true
  try {
    const rc = await axios.get('/api/class/my', auth)
    classes.value = rc.data || []
    // 并发拉每个班的考试
    const map = {}
    await Promise.all(classes.value.map(async (c) => {
      try {
        const re = await axios.get(`/api/class/${c.id}/exams`, auth)
        map[c.id] = re.data || []
      } catch (e) {
        map[c.id] = []
      }
    }))
    examsByClass.value = map
  } catch (e) {
    ElMessage.error('加载失败')
  } finally {
    loading.value = false
  }
}

function openExam(id) {
  router.push(`/exam/${id}`)
}

async function createExam() {
  if (!form.value.classId || !form.value.title || !form.value.startAt || !form.value.endAt) {
    ElMessage.warning('请填写班级、标题、开始/结束时间')
    return
  }
  try {
    await axios.post(`/api/class/${form.value.classId}/exam`, {
      title: form.value.title,
      description: form.value.description,
      startAt: form.value.startAt,
      endAt: form.value.endAt,
      durationMinutes: form.value.durationMinutes,
      totalScore: form.value.totalScore,
    }, auth)
    ElMessage.success('考试已创建')
    showCreate.value = false
    form.value = { classId: null, title: '', description: '', startAt: '', endAt: '', durationMinutes: 60, totalScore: 100 }
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '创建失败')
  }
}

onMounted(load)
</script>

<template>
  <div class="exam-list-page" v-loading="loading">
    <div class="header">
      <h1>考试中心</h1>
      <el-button type="primary" @click="showCreate = true">创建考试</el-button>
    </div>

    <el-empty v-if="!classes.length" description="你还没有加入任何班级" />

    <div v-for="c in classes" :key="c.id" class="class-block">
      <h3>{{ c.name }}</h3>
      <el-table v-if="examsByClass[c.id]?.length" :data="examsByClass[c.id]" size="small" @row-click="row => openExam(row.id)">
        <el-table-column prop="title" label="考试" />
        <el-table-column prop="totalScore" label="总分" width="80" />
        <el-table-column prop="durationMinutes" label="时长(分)" width="100" />
        <el-table-column label="开始时间" width="180">
          <template #default="{ row }">{{ row.startAt?.replace('T', ' ').slice(0, 16) }}</template>
        </el-table-column>
        <el-table-column label="结束时间" width="180">
          <template #default="{ row }">{{ row.endAt?.replace('T', ' ').slice(0, 16) }}</template>
        </el-table-column>
      </el-table>
      <el-empty v-else :image-size="40" description="暂无考试" />
    </div>

    <el-dialog v-model="showCreate" title="创建考试" width="500px">
      <el-form :model="form" label-width="90px">
        <el-form-item label="班级">
          <el-select v-model="form.classId" placeholder="选择班级">
            <el-option v-for="c in classes" :key="c.id" :label="c.name" :value="c.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="标题"><el-input v-model="form.title" /></el-form-item>
        <el-form-item label="描述"><el-input v-model="form.description" type="textarea" /></el-form-item>
        <el-form-item label="开始时间">
          <el-date-picker v-model="form.startAt" type="datetime" value-format="YYYY-MM-DDTHH:mm" />
        </el-form-item>
        <el-form-item label="结束时间">
          <el-date-picker v-model="form.endAt" type="datetime" value-format="YYYY-MM-DDTHH:mm" />
        </el-form-item>
        <el-form-item label="时长(分钟)"><el-input-number v-model="form.durationMinutes" :min="10" :max="300" /></el-form-item>
        <el-form-item label="总分"><el-input-number v-model="form.totalScore" :min="10" :max="100" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" @click="createExam">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.exam-list-page { padding: 24px; max-width: 1000px; margin: 0 auto; }
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
.class-block { margin-bottom: 24px; }
.class-block h3 { margin: 0 0 10px; }
</style>

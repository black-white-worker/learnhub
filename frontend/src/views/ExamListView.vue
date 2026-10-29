<script setup>
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import axios from 'axios'

const router = useRouter()
const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }
const userRole = localStorage.getItem('role') || ''
const canCreateExam = userRole === 'teacher' || userRole === 'admin'

const exams = ref([])
const pending = ref(0)
const classes = ref([])   // 只有老师建考试时才需要
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
    // 一次请求拿到「我所在班级的全部考试 + 我的提交状态 + 时间阶段」。
    // 改造前是「取我的班级 → 逐个班级取考试」，5 个班 6 次往返。
    const r = await axios.get('/api/my/exams', auth)
    exams.value = r.data?.list || []
    pending.value = r.data?.pending || 0
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '加载考试失败')
  } finally {
    loading.value = false
  }
}

async function loadClasses() {
  if (!canCreateExam) return
  try {
    const rc = await axios.get('/api/class/my', auth)
    classes.value = rc.data || []
  } catch (e) {
    classes.value = []
  }
}

// 按班级分组展示（保持原来的分班级视图，但数据仍是一次请求拿到的）
const groups = computed(() => {
  const map = new Map()
  for (const e of exams.value) {
    const key = e.className || '未命名班级'
    if (!map.has(key)) map.set(key, [])
    map.get(key).push(e)
  }
  return Array.from(map, ([className, items]) => ({ className, items }))
})

/**
 * 状态是两条轴拼出来的，不能只看"有没有交卷"：
 *   时间轴：phase = upcoming / ongoing / ended（服务端按服务器时间算）
 *   参与轴：myStatus = '' / started / submitted / expired
 * 旧实现只用了 myStatus，'' 一律显示"未开始"，
 * 结果已结束的考试也写着"未开始"。
 */
function statusOf(row) {
  if (row.myStatus === 'submitted') return { text: '已交卷', type: 'success' }
  if (row.myStatus === 'expired') return { text: '超时收卷', type: 'warning' }
  if (row.myStatus === 'started') return { text: '考试中', type: 'primary' }
  if (row.phase === 'upcoming') return { text: '未开始', type: 'info' }
  if (row.phase === 'ongoing') {
    // 老师/管理员不参加考试，别显示"待参加"
    return row.myRole === 'student'
      ? { text: '待参加', type: 'warning' }
      : { text: '进行中', type: 'primary' }
  }
  return { text: '已结束', type: 'info' }
}

// 只有学生在进行中的考试才给"去考试"按钮
function canEnter(row) {
  return row.canAttend === true
}

function openExam(id) {
  router.push(`/exam/${id}`)
}

function fmtTime(t) {
  if (!t) return '-'
  return String(t).replace('T', ' ').slice(0, 16)
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

onMounted(() => { load(); loadClasses() })
</script>

<template>
  <div class="exam-list-page" v-loading="loading">
    <div class="header">
      <div>
        <h1>考试中心</h1>
        <p class="sub" v-if="pending">有 {{ pending }} 场考试正在进行，别忘了参加</p>
      </div>
      <el-button v-if="canCreateExam" type="primary" @click="showCreate = true">创建考试</el-button>
    </div>

    <el-empty v-if="!loading && !exams.length" description="你所在的班级还没有考试" />

    <!-- 分班级展示：每个班一块 -->
    <div v-for="g in groups" :key="g.className" class="class-block">
      <div class="class-head">
        <h3>{{ g.className }}</h3>
        <span class="count">{{ g.items.length }} 场</span>
      </div>
      <el-table :data="g.items" size="small">
        <el-table-column prop="title" label="考试" min-width="180" />
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="statusOf(row).type">{{ statusOf(row).text }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="我的分数" width="110">
          <template #default="{ row }">
            <span v-if="row.myStatus === 'submitted' || row.myStatus === 'expired'">
              {{ row.myScore !== null && row.myScore !== undefined ? row.myScore : '-' }} / {{ row.totalScore }}
            </span>
            <span v-else style="color:#c0c4cc">-</span>
          </template>
        </el-table-column>
        <el-table-column label="开考时间" width="160">
          <template #default="{ row }">{{ fmtTime(row.startAt) }}</template>
        </el-table-column>
        <el-table-column label="结束时间" width="160">
          <template #default="{ row }">{{ fmtTime(row.endAt) }}</template>
        </el-table-column>
        <el-table-column prop="durationMinutes" label="时长(分)" width="90" />
        <el-table-column label="操作" width="110">
          <template #default="{ row }">
            <el-button v-if="canEnter(row)" size="small" type="primary" @click="openExam(row.id)">
              去考试
            </el-button>
            <el-button v-else size="small" @click="openExam(row.id)">查看</el-button>
          </template>
        </el-table-column>
      </el-table>
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
.exam-list-page { padding: 24px; max-width: 1080px; margin: 0 auto; }
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
.header h1 { font-size: 22px; margin: 0; color: #1f2937; }
.header .sub { margin: 4px 0 0; font-size: 13px; color: #f59e0b; }
.class-block {
  margin-bottom: 18px;
  background: #fff;
  border-radius: 12px;
  padding: 16px;
  box-shadow: 0 1px 4px rgba(0,0,0,0.05);
}
.class-head { display: flex; align-items: baseline; gap: 8px; margin-bottom: 8px; }
.class-head h3 { margin: 0; font-size: 16px; color: #1f2937; }
.class-head .count { font-size: 12px; color: #94a3b8; }
</style>

<script setup>
// QuestionEditor 公共出题器：考试和作业共用一套组件。
// 三栏布局：左题型栏 / 中间题目卡 / 右整卷设置。
// 通过 props 控制可用题型和是否显示分值，考试和作业复用。
import { ref, computed, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import axios from 'axios'

const props = defineProps({
  // 模块：exam 或 assignment
  module: { type: String, default: 'exam' },
  // 资源 id：examId 或 assignmentId
  resourceId: { type: [Number, String], required: true },
  // 初始题目列表
  initialQuestions: { type: Array, default: () => [] },
  // 初始整卷设置
  initialSettings: { type: Object, default: () => ({}) },
  // 可用题型列表：exam 默认 ['single','multiple','judge','fill','essay']
  // assignment 默认 ['single','multiple','fill','essay','fileupload']
  availableTypes: { type: Array, default: () => ['single', 'multiple', 'judge', 'fill', 'essay'] },
  // 是否显示分值（考试显示，作业也显示）
  showScore: { type: Boolean, default: true },
  // 学生端模式：true 时只渲染题目供作答，不显示编辑按钮
  isStudent: { type: Boolean, default: false },
})

const emit = defineEmits(['saved', 'submit', 'settings-saved'])

const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }

// 题目列表（本地状态）
const questions = ref([])
// 整卷设置
const settings = ref({
  title: '',
  description: '',
  startAt: '',
  endAt: '',
  durationMinutes: 60,
  totalScore: 100,
  allowLate: 0,
  tabLimit: 0,
  deadline: '',
  lateDeadline: '',
  ...props.initialSettings,
})

// 题型元数据
const typeMeta = {
  single:    { label: '单选题', icon: '○' },
  multiple:  { label: '多选题', icon: '□' },
  judge:     { label: '判断题', icon: '✓✗' },
  fill:      { label: '填空题', icon: '___' },
  essay:     { label: '简答题', icon: '¶' },
  fileupload:{ label: '文件上传', icon: '📎' },
}

onMounted(() => {
  questions.value = (props.initialQuestions || []).map((q, i) => normalizeQuestion(q, i))
})

watch(() => props.initialQuestions, (val) => {
  if (val && val.length && !questions.value.length) {
    questions.value = val.map((q, i) => normalizeQuestion(q, i))
  }
})

function normalizeQuestion(q, i) {
  let options = []
  try {
    options = q.options ? JSON.parse(q.options) : []
  } catch (e) { options = [] }
  if (q.type === 'judge' && !options.length) options = ['对', '错']
  return {
    id: q.id,
    type: q.type,
    stem: q.stem || '',
    options,
    answer: q.answer || '',
    score: q.score || 5,
    required: q.required === undefined ? 1 : q.required,
    orderNum: q.orderNum || i + 1,
    _new: !q.id,
  }
}

// 总分自动累加
const totalScore = computed(() => questions.value.reduce((s, q) => s + (Number(q.score) || 0), 0))

// API 路径前缀
function api(p) {
  if (props.module === 'exam') return `/api/exam${p}`
  return `/api/assignment${p}`
}

// 左侧题型栏点击：追加一张空题卡
async function addQuestion(type) {
  try {
    const r = await axios.post(api(`/${props.resourceId}/question`), { type, stem: '', score: 5 }, auth)
    const newQ = {
      id: r.data.id, type, stem: '', options: type === 'judge' ? ['对', '错'] : [],
      answer: '', score: 5, required: 1, orderNum: questions.value.length + 1, _new: false,
    }
    questions.value.push(newQ)
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '添加题目失败')
  }
}

// 题目字段变化时即时保存（debounce 由调用方控制，这里直接调）
let saveTimers = {}
function scheduleSave(q) {
  clearTimeout(saveTimers[q.id])
  saveTimers[q.id] = setTimeout(() => saveQuestion(q), 600)
}

async function saveQuestion(q) {
  try {
    const body = {
      type: q.type,
      stem: q.stem,
      options: ['single', 'multiple'].includes(q.type) ? JSON.stringify(q.options) : '',
      answer: q.answer,
      score: q.score,
      required: q.required,
    }
    await axios.put(api(`/question/${q.id}`), body, auth)
  } catch (e) {
    ElMessage.error('保存失败：' + (e.response?.data?.message || e.message))
  }
}

async function removeQuestion(q) {
  try {
    await ElMessageBox.confirm('确定删除这道题？', '提示', { type: 'warning' })
  } catch { return }
  try {
    await axios.delete(api(`/question/${q.id}`), auth)
    questions.value = questions.value.filter(x => x.id !== q.id)
    ElMessage.success('已删除')
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '删除失败')
  }
}

function duplicateQuestion(q) {
  const copy = JSON.parse(JSON.stringify(q))
  copy.id = null
  copy._new = true
  questions.value.push(copy)
  // TODO: 后端复制接口暂不做，前端先复制本地，下次 saveQuestion 走 PUT 时会因为 id 为空失败
  // 这里简单处理：直接调 addQuestion 再改
  addQuestion(q.type).then(() => {
    const last = questions.value[questions.value.length - 1]
    last.stem = q.stem
    last.options = [...q.options]
    last.answer = q.answer
    last.score = q.score
    scheduleSave(last)
  })
}

function moveQuestion(index, dir) {
  const target = index + dir
  if (target < 0 || target >= questions.value.length) return
  const arr = [...questions.value]
  ;[arr[index], arr[target]] = [arr[target], arr[index]]
  questions.value = arr
  saveSort(arr)
}

async function saveSort(arr) {
  // 用第一个题的 id 反查 resourceId（路由约定）
  try {
    const order = arr.map(q => q.id).filter(Boolean)
    if (order.length < 2) return
    await axios.put(api(`/question/${order[0]}/sort`), { order }, auth)
  } catch (e) {
    // 排序失败不打扰用户
  }
}

// 拖拽排序
let dragIndex = null
function onDragStart(i) { dragIndex = i }
function onDrop(i) {
  if (dragIndex === null || dragIndex === i) return
  const arr = [...questions.value]
  const [moved] = arr.splice(dragIndex, 1)
  arr.splice(i, 0, moved)
  questions.value = arr
  dragIndex = null
  saveSort(arr)
}

function addOption(q) {
  q.options.push('')
  scheduleSave(q)
}
function removeOption(q, idx) {
  q.options.splice(idx, 1)
  scheduleSave(q)
}

async function saveSettings() {
  try {
    const body = { ...settings.value }
    if (props.module === 'exam') {
      await axios.put(api(`/${props.resourceId}`), body, auth)
    } else {
      await axios.put(api(`/${props.resourceId}`), {
        title: body.title, description: body.description,
        deadline: body.deadline, lateDeadline: body.lateDeadline,
        totalScore: body.totalScore, allowLate: body.allowLate,
      }, auth)
    }
    ElMessage.success('整卷设置已保存')
    emit('settings-saved', settings.value)
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '保存失败')
  }
}

// 暴露给父组件：学生端用
defineExpose({ questions, settings })
</script>

<template>
  <div class="qe-container">
    <!-- 左：题型栏 -->
    <div class="qe-left">
      <div class="qe-left-title">题型</div>
      <div
        v-for="t in availableTypes"
        :key="t"
        class="qe-type-item"
        @click="!isStudent && addQuestion(t)"
      >
        <span class="qe-type-icon">{{ typeMeta[t].icon }}</span>
        <span>{{ typeMeta[t].label }}</span>
      </div>
    </div>

    <!-- 中：题目卡片 -->
    <div class="qe-center">
      <div v-if="!questions.length" class="qe-empty">
        点击左侧题型添加题目
      </div>
      <div
        v-for="(q, i) in questions"
        :key="q.id || ('new-' + i)"
        class="qe-card"
        draggable="true"
        @dragstart="onDragStart(i)"
        @dragover.prevent
        @drop="onDrop(i)"
      >
        <div class="qe-card-head">
          <span class="qe-no">{{ String(i + 1).padStart(2, '0') }}</span>
          <span v-if="q.required" class="qe-star">*</span>
          <el-tag size="small">{{ typeMeta[q.type]?.label || q.type }}</el-tag>
          <div class="qe-head-actions">
            <template v-if="!isStudent">
              <el-input-number v-model="q.score" :min="1" :max="100" size="small" controls-position="right" style="width:90px" @change="scheduleSave(q)" />
              <span style="font-size:12px;color:#888">分</span>
              <el-switch v-model="q.required" :active-value="1" :inactive-value="0" active-text="必答" @change="scheduleSave(q)" />
              <el-button size="small" link @click="moveQuestion(i, -1)" :disabled="i===0">↑</el-button>
              <el-button size="small" link @click="moveQuestion(i, 1)" :disabled="i===questions.length-1">↓</el-button>
              <el-button size="small" link @click="duplicateQuestion(q)">复制</el-button>
              <el-button size="small" link type="danger" @click="removeQuestion(q)">删除</el-button>
            </template>
          </div>
        </div>

        <el-input
          v-model="q.stem"
          type="textarea"
          :rows="2"
          placeholder="请输入题干"
          @input="scheduleSave(q)"
        />

        <!-- 选项区：单选/多选 -->
        <div v-if="['single','multiple'].includes(q.type)" class="qe-options">
          <div v-for="(opt, oi) in q.options" :key="oi" class="qe-option-row">
            <span class="qe-option-letter">{{ 'ABCD'[oi] }}</span>
            <el-input v-model="q.options[oi]" placeholder="选项内容" @input="scheduleSave(q)" />
            <el-button size="small" link type="danger" @click="removeOption(q, oi)">删</el-button>
          </div>
          <el-button size="small" @click="addOption(q)">+ 添加选项</el-button>
        </div>

        <!-- 判断题自动对/错 -->
        <div v-else-if="q.type === 'judge'" class="qe-judge">
          <el-radio-group v-model="q.answer" @change="scheduleSave(q)">
            <el-radio value="true">对</el-radio>
            <el-radio value="false">错</el-radio>
          </el-radio-group>
        </div>

        <!-- 答案设置（老师端） -->
        <div v-if="!isStudent && ['single','multiple','fill'].includes(q.type)" class="qe-answer">
          <el-input
            v-model="q.answer"
            :placeholder="q.type==='multiple' ? '填选项大写字母，逗号分隔，如 A,C' : (q.type==='single' ? '填选项大写字母，如 A' : '填标准答案')"
            @input="scheduleSave(q)"
            size="small"
            style="max-width:320px"
          />
          <span style="font-size:12px;color:#888;margin-left:8px">标准答案</span>
        </div>
      </div>
    </div>

    <!-- 右：整卷设置 -->
    <div class="qe-right">
      <div class="qe-right-title">整卷设置</div>
      <el-form label-width="80px" size="small">
        <el-form-item label="标题">
          <el-input v-model="settings.title" />
        </el-form-item>
        <el-form-item label="说明">
          <el-input v-model="settings.description" type="textarea" :rows="3" />
        </el-form-item>
        <template v-if="module === 'exam'">
          <el-form-item label="开始时间">
            <el-date-picker v-model="settings.startAt" type="datetime" value-format="YYYY-MM-DDTHH:mm" />
          </el-form-item>
          <el-form-item label="截止时间">
            <el-date-picker v-model="settings.endAt" type="datetime" value-format="YYYY-MM-DDTHH:mm" />
          </el-form-item>
          <el-form-item label="时长(分)">
            <el-input-number v-model="settings.durationMinutes" :min="1" :max="300" />
          </el-form-item>
          <el-form-item label="切屏限制">
            <el-input-number v-model="settings.tabLimit" :min="0" :max="20" />
          </el-form-item>
          <el-form-item label="允许迟到">
            <el-switch v-model="settings.allowLate" :active-value="1" :inactive-value="0" />
          </el-form-item>
        </template>
        <template v-else>
          <el-form-item label="截止时间">
            <el-date-picker v-model="settings.deadline" type="datetime" value-format="YYYY-MM-DDTHH:mm" />
          </el-form-item>
          <el-form-item label="允许补交">
            <el-switch v-model="settings.allowLate" :active-value="1" :inactive-value="0" />
          </el-form-item>
          <el-form-item label="补交截止">
            <el-date-picker v-model="settings.lateDeadline" type="datetime" value-format="YYYY-MM-DDTHH:mm" />
          </el-form-item>
        </template>
        <el-form-item label="总分">
          <el-input :model-value="totalScore" disabled />
        </el-form-item>
        <el-button type="primary" @click="saveSettings">保存设置</el-button>
      </el-form>
    </div>
  </div>
</template>

<style scoped>
/* ===== 三栏编辑器布局（参考腾讯问卷 / 问卷星）=====
 *
 * 关键点：让编辑器自己成为滚动容器，只有中间"题目列"滚动，
 * 左边题型栏和右边整卷设置始终钉在原地。
 *
 * 为什么不继续用 position: sticky：
 *   外层 Element Plus 的 <el-main> 自带 overflow: auto，且高度随内容撑开。
 *   它因此成为一个"永远不滚动的滚动容器"，里面的 sticky 元素的粘附参照系
 *   就是它自己 —— 于是 sticky 永远不会触发。这是上一版改完没生效的真正原因。
 *   改成固定高度 + 内部滚动后，不再依赖任何祖先的滚动行为。
 */
.qe-container {
  display: flex;
  gap: 12px;
  align-items: stretch;
  /* 顶部导航 64 + 教师头 ~50 + 页脚 44，留一点余量；dvh 兼容移动端地址栏 */
  height: calc(100vh - 170px);
  height: calc(100dvh - 170px);
  min-height: 420px;
  box-sizing: border-box;
  padding: 12px;
}
/* 左：题型栏。自身可滚动（题型多时不会撑破），但位置不随中间内容移动 */
.qe-left {
  width: 140px;
  flex-shrink: 0;
  background: #fafafa;
  border-radius: 8px;
  padding: 12px;
  height: 100%;
  overflow-y: auto;
}
.qe-left-title {
  font-weight: 600;
  margin-bottom: 12px;
  color: #333;
  position: sticky;
  top: 0;
  background: #fafafa;
  padding-bottom: 4px;
}
.qe-type-item { padding: 10px; border-radius: 6px; cursor: pointer; display: flex; align-items: center; gap: 8px; margin-bottom: 6px; transition: background .15s; }
.qe-type-item:hover { background: #e8f0fe; }
.qe-type-icon { font-size: 16px; width: 24px; text-align: center; }
/* 中：唯一滚动的列 */
.qe-center {
  flex: 1;
  min-width: 0;
  height: 100%;
  overflow-y: auto;
  padding-right: 4px;
}
.qe-empty { text-align: center; color: #999; padding: 60px 0; }
.qe-card { background: #fff; border: 1px solid #eaeaea; border-radius: 8px; padding: 14px; margin-bottom: 12px; }
.qe-card-head { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; }
.qe-no { font-weight: 700; color: #409eff; }
.qe-star { color: #f56c6c; }
.qe-head-actions { margin-left: auto; display: flex; align-items: center; gap: 6px; }
.qe-options { margin-top: 10px; }
.qe-option-row { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.qe-option-letter { color: #666; width: 20px; }
.qe-judge { margin-top: 10px; }
.qe-answer { margin-top: 10px; }
/* 右：整卷设置，同样固定、自身可滚 */
.qe-right {
  width: 280px;
  flex-shrink: 0;
  background: #fafafa;
  border-radius: 8px;
  padding: 16px;
  height: 100%;
  overflow-y: auto;
}
.qe-right-title { font-weight: 600; margin-bottom: 12px; position: sticky; top: 0; background: #fafafa; padding-bottom: 4px; }

/* 窄屏：改为纵向堆叠，避免三栏挤在一起 */
@media (max-width: 1100px) {
  .qe-container { flex-direction: column; height: auto; }
  .qe-left { width: 100%; height: auto; }
  .qe-right { width: 100%; height: auto; }
  .qe-center { height: auto; overflow: visible; }
}
</style>

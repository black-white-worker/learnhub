<script setup>
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { createCourse, fetchCourseList, uploadCourseCover, uploadCourseVideos } from '../api/index.js'
import { uploadCoverFlow, uploadVideosFlow, createCourseFlow } from '../utils/courseCreateFlow.js'

const router = useRouter()
const formRef = ref(null)
const loading = ref(false)

// 表单数据：课程名称必填，简介和教师可选
const form = reactive({
  name: '',
  intro: '',
  teacher: '',
})

// 前端重复名校验：失焦时拉取课程列表，命中同名就拦截，避免白等一次后端请求
// 例外：如果同名课程就是"当前刚创建成功的那门"（上传失败重试场景），放行
async function validateCourseName(rule, value, callback) {
  if (!value) return callback()
  try {
    const res = await fetchCourseList()
    const exists = (res.data.courses || []).some((c) => c.name === value)
    if (exists && createdName.value !== value) {
      callback(new Error('该课程名称已存在，请换一个'))
    } else {
      callback()
    }
  } catch {
    // 列表接口暂时不可用时放行，后端仍会兜底校验重名
    callback()
  }
}

// Element Plus 表单校验规则
const rules = {
  name: [
    { required: true, message: '课程名称不能为空', trigger: 'blur' },
    { min: 2, max: 50, message: '课程名称长度 2-50 个字符', trigger: 'blur' },
    { validator: validateCourseName, trigger: 'blur' },
  ],
  intro: [{ max: 200, message: '简介最长 200 个字符', trigger: 'blur' }],
  teacher: [{ max: 20, message: '教师名最长 20 个字符', trigger: 'blur' }],
}

// ===== 封面 / 视频上传状态 =====
const createdId = ref(0)      // 创建成功后记录课程 id，上传接口靠它定位课程
const createdName = ref('')   // 当前课程的名称（展示用）
const coverFile = ref(null)   // 待上传的封面文件（File 对象）
const coverUploading = ref(false)
const coverResult = ref(null) // 上传成功后的 {coverUrl, coverThumbUrl}
const videoFiles = ref([])    // 待上传的视频文件列表（File 对象数组）
const videoUploading = ref(false)
const videoResult = ref([])   // 上传成功后的视频列表 [{id, name, url, size, uploadedAt}]

// ===== 传给流程函数的接口与状态容器 =====
// 状态容器持有各 ref 的引用，流程函数内部直接改 .value，
// 组件里的变量名保持原样（createdId / coverResult / videoResult ...）。
const api = { createCourse, fetchCourseList, uploadCourseCover, uploadCourseVideos }
const session = {
  createdId,
  createdName,
  coverResult,
  videoResult,
}

// ===== 主提交：创建课程 =====
// 流程：确保课程存在（未创建则先创建）→ 兜底上传"选了但没传"的文件
// 全部成功 → 跳转课程列表；任一步失败 → 留在页面提示对应错误
async function handleCreate() {
  // 先触发 Element Plus 表单校验，没通过就直接返回，不发请求
  try {
    await formRef.value.validate()
  } catch {
    return
  }

  loading.value = true
  const result = await createCourseFlow(
    api,
    form,
    session,
    {
      coverFile: coverFile.value,
      videoFiles: videoFiles.value,
      coverResult,
      videoResult,
    },
  )
  loading.value = false

  if (result.ok) {
    ElMessage.success('课程创建成功，正在跳转课程列表')
    setTimeout(() => router.push('/courses'), 1000)
  } else {
    // 失败留在页面，提示后端返回的具体错误（如"课程名称已存在"）
    ElMessage.error(result.error)
  }
}

// ===== 封面上传（单独重试用）=====
function onCoverChange(file) {
  coverFile.value = file.raw
  coverResult.value = null // 换了文件后旧结果作废
}

async function handleUploadCover() {
  // 上传封面前先校验"有没有选文件"；没选时提示先选文件
  if (!coverFile.value) {
    ElMessage.warning('请先选择封面图片')
    return
  }
  coverUploading.value = true
  // 课程未创建时流程内部会用当前表单数据自动创建，再上传封面
  const result = await uploadCoverFlow(api, form, session, coverFile.value)
  coverUploading.value = false

  if (result.ok) {
    ElMessage.success('封面上传成功，可继续上传视频')
  } else {
    // 失败留在页面，提示对应错误（可能是创建失败或上传失败）
    ElMessage.error(result.error)
  }
}

// ===== 多视频上传（单独重试用）=====
function onVideosChange(file, fileList) {
  // fileList 是 el-upload 内部维护的列表，取每个的 .raw 才是真正的 File 对象
  videoFiles.value = fileList.map((f) => f.raw)
  videoResult.value = []
}

async function handleUploadVideos() {
  // 上传视频前先校验"有没有选文件"；没选时提示先选文件
  if (videoFiles.value.length === 0) {
    ElMessage.warning('请先选择视频文件')
    return
  }
  videoUploading.value = true
  // 课程未创建时流程内部会用当前表单数据自动创建，再上传视频
  const result = await uploadVideosFlow(api, form, session, videoFiles.value)
  videoUploading.value = false

  if (result.ok) {
    ElMessage.success(`成功上传 ${result.count} 个视频`)
  } else {
    // 失败留在页面，提示对应错误
    ElMessage.error(result.error)
  }
}

// 把文件字节数格式化为人类可读的 KB/MB/GB
function formatSize(bytes) {
  if (bytes == null) return '-'
  const units = ['B', 'KB', 'MB', 'GB']
  let i = 0
  let n = Number(bytes)
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024
    i++
  }
  return `${n.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}
</script>

<template>
  <div class="course-create-page">
    <el-card class="course-create-card">
      <h2 class="title">新建课程</h2>

      <!-- 基本信息 -->
      <el-form ref="formRef" :model="form" :rules="rules" label-width="76px">
        <el-form-item label="课程名称" prop="name">
          <el-input v-model="form.name" placeholder="请输入课程名称（不能与已有课程重名）" />
        </el-form-item>
        <el-form-item label="简介" prop="intro">
          <el-input v-model="form.intro" type="textarea" :rows="3" placeholder="请输入课程简介（可选）" />
        </el-form-item>
        <el-form-item label="教师" prop="teacher">
          <el-input v-model="form.teacher" placeholder="请输入授课教师（可选）" />
        </el-form-item>
      </el-form>

      <!-- 封面：选择随时可用；未创建课程时上传按钮点击会提示先创建 -->
      <el-form label-width="76px">
        <el-form-item label="课程封面">
          <el-upload accept="image/*" :auto-upload="false" :on-change="onCoverChange">
            <el-button>选择封面图片</el-button>
            <template #tip>
              <div class="el-upload__tip">支持 jpg / png / webp，仅一张</div>
            </template>
          </el-upload>
        </el-form-item>
        <el-form-item>
          <el-button type="success" :loading="coverUploading" @click="handleUploadCover">
            上传封面
          </el-button>
        </el-form-item>
        <el-form-item v-if="coverResult" label="封面预览">
          <div class="cover-preview">
            <img :src="coverResult.coverThumbUrl" alt="封面缩略图" />
            <p>原图：<a :href="coverResult.coverUrl" target="_blank" rel="noopener">查看</a></p>
          </div>
        </el-form-item>
      </el-form>

      <!-- 视频：选择随时可用 -->
      <el-form label-width="76px">
        <el-form-item label="课程视频">
          <el-upload
            accept="video/*"
            multiple
            :auto-upload="false"
            :on-change="onVideosChange"
          >
            <el-button>选择视频文件</el-button>
            <template #tip>
              <div class="el-upload__tip">支持一次选择多个视频（mp4 / mov 等）</div>
            </template>
          </el-upload>
        </el-form-item>
        <el-form-item>
          <el-button type="success" :loading="videoUploading" @click="handleUploadVideos">
            上传视频
          </el-button>
        </el-form-item>
      </el-form>

      <!-- 上传成功后的视频列表 -->
      <el-table v-if="videoResult.length" :data="videoResult" border size="small" empty-text="暂无视频">
        <el-table-column prop="name" label="文件名" min-width="200" show-overflow-tooltip />
        <el-table-column label="大小" width="100">
          <template #default="{ row }">{{ formatSize(row.size) }}</template>
        </el-table-column>
        <el-table-column label="链接" width="110">
          <template #default="{ row }">
            <a :href="row.url" target="_blank" rel="noopener">播放/下载</a>
          </template>
        </el-table-column>
      </el-table>

      <!-- 创建课程按钮：放在所有功能块下方，作为总提交 -->
      <div class="create-btn-row">
        <el-button type="primary" :loading="loading" style="width: 100%" @click="handleCreate">
          创建课程
        </el-button>
      </div>
      <p class="tip">选择文件后点击"上传封面 / 上传视频"可单独上传并查看结果；最后点击"创建课程"，成功后跳转课程列表，失败留在本页可重试</p>
    </el-card>
  </div>
</template>

<style scoped>
.course-create-page {
  display: flex;
  justify-content: center;
  align-items: flex-start;
  min-height: 70vh;
  background: linear-gradient(160deg, #eef2ff 0%, #f5f7fa 60%);
  padding: 24px;
}

.course-create-card {
  width: 560px;
  padding: 12px 16px;
  border-radius: 14px;
  box-shadow: 0 12px 32px rgba(37, 99, 235, 0.12);
}

.title {
  text-align: center;
  margin: 0 0 16px;
}

.cover-preview img {
  max-width: 320px;
  border-radius: 8px;
  display: block;
  margin-bottom: 8px;
}

.create-btn-row {
  margin-top: 16px;
}

.tip {
  color: #999;
  font-size: 13px;
  text-align: center;
  margin: 10px 0 0;
}
</style>

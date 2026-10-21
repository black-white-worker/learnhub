<script setup>
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { createCourse, uploadCourseCover, uploadCourseVideos } from '../api/index.js'

const router = useRouter()
const formRef = ref(null)
const loading = ref(false)

// 表单数据：课程名称必填，简介和教师可选
const form = reactive({
  name: '',
  intro: '',
  teacher: '',
})

// Element Plus 表单校验规则
const rules = {
  name: [
    { required: true, message: '课程名称不能为空', trigger: 'blur' },
    { min: 2, max: 50, message: '课程名称长度 2-50 个字符', trigger: 'blur' },
  ],
  intro: [{ max: 200, message: '简介最长 200 个字符', trigger: 'blur' }],
  teacher: [{ max: 20, message: '教师名最长 20 个字符', trigger: 'blur' }],
}

// ===== 封面上传 + 视频上传（MinIO）状态 =====
const createdId = ref(0)      // 创建成功后记录课程 id，上传接口靠它定位课程
const createdName = ref('')   // 当前课程的名称（展示用）
const coverFile = ref(null)   // 待上传的封面文件（File 对象）
const coverUploading = ref(false)
const coverResult = ref(null) // 上传成功后的 {coverUrl, coverThumbUrl}
const videoFiles = ref([])    // 待上传的视频文件列表（File 对象数组）
const videoUploading = ref(false)
const videoResult = ref([])   // 上传成功后的视频列表 [{id, name, url, size, uploadedAt}]

async function handleCreate() {
  // 先触发 Element Plus 表单校验，没通过就直接返回，不发请求
  try {
    await formRef.value.validate()
  } catch {
    return
  }

  loading.value = true
  try {
    // 调用后端课程创建接口
    const res = await createCourse({
      name: form.name,
      intro: form.intro,
      teacher: form.teacher,
    })
    // 记住创建出的课程，供后续上传封面/视频使用
    createdId.value = res.data.id
    createdName.value = res.data.name
    // 重置上传区状态，避免上一门课的残留
    coverFile.value = null
    coverResult.value = null
    videoFiles.value = []
    videoResult.value = []
    ElMessage.success(`课程创建成功：${res.data.name}`)
    // 创建成功后清空表单，方便继续录入下一门课程
    form.name = ''
    form.intro = ''
    form.teacher = ''
  } catch (e) {
    // 后端返回错误时，msg 字段是错误信息（如课程已存在）
    ElMessage.error(e.response?.data?.msg || '课程创建失败，请重试')
  } finally {
    loading.value = false
  }
}

// ===== 封面上传 =====
// el-upload 关闭自动上传（:auto-upload="false"），只在用户选择文件时把 File 存下来
function onCoverChange(file) {
  coverFile.value = file.raw
  coverResult.value = null // 换了文件后旧结果作废
}

async function handleUploadCover() {
  if (!createdId.value) {
    ElMessage.warning('请先创建课程')
    return
  }
  if (!coverFile.value) {
    ElMessage.warning('请先选择封面图片')
    return
  }
  // 组装 multipart 表单：courseId + file（字段名与后端 CoverUploadHandler 约定一致）
  const fd = new FormData()
  fd.append('courseId', createdId.value)
  fd.append('file', coverFile.value)

  coverUploading.value = true
  try {
    const res = await uploadCourseCover(fd)
    coverResult.value = res.data
    ElMessage.success('封面上传成功')
  } catch (e) {
    ElMessage.error(e.response?.data?.msg || '封面上传失败，请重试')
  } finally {
    coverUploading.value = false
  }
}

// ===== 多视频上传 =====
function onVideosChange(file, fileList) {
  // fileList 是 el-upload 内部维护的列表，取每个的 .raw 才是真正的 File 对象
  videoFiles.value = fileList.map((f) => f.raw)
  videoResult.value = []
}

async function handleUploadVideos() {
  if (!createdId.value) {
    ElMessage.warning('请先创建课程')
    return
  }
  if (videoFiles.value.length === 0) {
    ElMessage.warning('请先选择视频文件')
    return
  }
  // 组装 multipart 表单：courseId + 多个 files（字段名与后端 VideoUploadHandler 约定一致）
  const fd = new FormData()
  fd.append('courseId', createdId.value)
  videoFiles.value.forEach((v) => fd.append('files', v))

  videoUploading.value = true
  try {
    const res = await uploadCourseVideos(fd)
    videoResult.value = res.data.videos || []
    ElMessage.success(`成功上传 ${videoResult.value.length} 个视频`)
  } catch (e) {
    ElMessage.error(e.response?.data?.msg || '视频上传失败，请重试')
  } finally {
    videoUploading.value = false
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
      <el-form ref="formRef" :model="form" :rules="rules" label-width="76px">
        <el-form-item label="课程名称" prop="name">
          <el-input v-model="form.name" placeholder="请输入课程名称" />
        </el-form-item>
        <el-form-item label="简介" prop="intro">
          <el-input v-model="form.intro" type="textarea" :rows="3" placeholder="请输入课程简介（可选）" />
        </el-form-item>
        <el-form-item label="教师" prop="teacher">
          <el-input v-model="form.teacher" placeholder="请输入授课教师（可选）" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" style="width: 100%" @click="handleCreate">
            创建课程
          </el-button>
        </el-form-item>
      </el-form>
      <p class="tip">创建成功后清空表单，可继续录入下一门课程</p>
    </el-card>

    <!-- 课程创建成功后，出现封面上传 + 视频上传区（MinIO） -->
    <el-card v-if="createdId" class="upload-card">
      <h3 class="upload-title">为「{{ createdName }}」上传素材</h3>

      <!-- 封面：单文件，图片 -->
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
          <el-button
            type="success"
            :loading="coverUploading"
            :disabled="!coverFile"
            @click="handleUploadCover"
          >
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

      <!-- 视频：多文件 -->
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
          <el-button
            type="success"
            :loading="videoUploading"
            :disabled="videoFiles.length === 0"
            @click="handleUploadVideos"
          >
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
    </el-card>
  </div>
</template>

<style scoped>
.course-create-page {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 20px;
  min-height: 70vh;
  background: linear-gradient(160deg, #eef2ff 0%, #f5f7fa 60%);
  padding: 24px;
}

.course-create-card {
  width: 480px;
  padding: 12px 8px;
  border-radius: 14px;
  box-shadow: 0 12px 32px rgba(37, 99, 235, 0.12);
}

.upload-card {
  width: 560px;
  padding: 12px 8px;
  border-radius: 14px;
  box-shadow: 0 12px 32px rgba(37, 99, 235, 0.12);
}

.title,
.upload-title {
  text-align: center;
  margin: 0 0 16px;
}

.cover-preview img {
  max-width: 320px;
  border-radius: 8px;
  display: block;
  margin-bottom: 8px;
}

.tip {
  color: #999;
  font-size: 13px;
  text-align: center;
}
</style>

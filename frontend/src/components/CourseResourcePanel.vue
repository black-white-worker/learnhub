<script setup>
import { ref, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  fetchCourseResources,
  uploadCourseResource,
  deleteCourseResource,
} from '../api/index.js'

// 课程资源面板（来自「课程资源」分支）：
// 自包含组件，接收 courseId，自己负责加载 / 上传 / 删除资源列表。
// 既可以放在课程详情页的"课程资源"tab，也可以独立成页使用。
const props = defineProps({
  courseId: { type: [Number, String], required: true },
  // 是否允许上传 / 删除（课程创建者才传 true）
  canEdit: { type: Boolean, default: true },
})

const resources = ref([])
const loading = ref(false)
const uploading = ref(false)
const fileInput = ref(null)

async function load() {
  loading.value = true
  try {
    const res = await fetchCourseResources(props.courseId)
    resources.value = res.data.resources || []
  } catch (e) {
    ElMessage.error('资源加载失败：' + (e.response?.data?.msg || e.message))
  } finally {
    loading.value = false
  }
}

onMounted(load)
watch(() => props.courseId, load)

// 选好文件后立即上传
function onChange(file) {
  const f = file.raw || file
  doUpload(f)
}

async function doUpload(file) {
  if (!file) return
  uploading.value = true
  const fd = new FormData()
  fd.append('file', file)
  try {
    await uploadCourseResource(props.courseId, fd)
    ElMessage.success('资源上传成功')
    load()
  } catch (e) {
    ElMessage.error('上传失败：' + (e.response?.data?.msg || e.message))
  } finally {
    uploading.value = false
    if (fileInput.value) fileInput.value.clearFiles()
  }
}

async function onDelete(res) {
  try {
    await ElMessageBox.confirm(`确定删除资源「${res.name}」吗？`, '删除资源', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  try {
    await deleteCourseResource(props.courseId, res.id)
    ElMessage.success('已删除')
    load()
  } catch (e) {
    ElMessage.error('删除失败：' + (e.response?.data?.msg || e.message))
  }
}

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

// 按类型给一个颜色标签
function typeTag(type) {
  switch (type) {
    case 'pdf': return { text: 'PDF', color: '#dc2626' }
    case 'ppt': return { text: 'PPT', color: '#ea580c' }
    case 'doc': return { text: 'DOC', color: '#2563eb' }
    default: return { text: 'FILE', color: '#6b7280' }
  }
}
</script>

<template>
  <div class="resource-panel" v-loading="loading">
    <div class="panel-head">
      <span class="tip">支持 PDF / PPT / Word 讲义</span>
      <el-upload
        v-if="canEdit"
        ref="fileInput"
        :auto-upload="false"
        :show-file-list="false"
        accept=".pdf,.ppt,.pptx,.doc,.docx"
        :on-change="onChange"
      >
        <el-button type="primary" :loading="uploading">上传资源</el-button>
      </el-upload>
    </div>

    <div v-if="resources.length" class="res-list">
      <div v-for="r in resources" :key="r.id" class="res-item">
        <span class="type-badge" :style="{ background: typeTag(r.type).color }">
          {{ typeTag(r.type).text }}
        </span>
        <div class="res-meta">
          <span class="res-name">{{ r.name }}</span>
          <span class="res-sub">{{ formatSize(r.size) }} · {{ r.uploadedAt }}</span>
        </div>
        <a :href="`/api/course/resource/preview/${courseId}/${r.id}`" target="_blank" rel="noopener" class="res-btn">预览</a>
        <a :href="r.url" :download="r.name" class="res-btn res-btn-download">下载</a>
        <el-button v-if="canEdit" size="small" type="primary" link @click="onDelete(r)">删除</el-button>
      </div>
    </div>
    <el-empty v-else-if="!loading" description="暂无资源，上传课件讲义吧" />
  </div>
</template>

<style scoped>
.resource-panel {
  padding: 4px 0;
}

.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
}

.tip {
  font-size: 13px;
  color: #9ca3af;
}

.res-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.res-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid #eef0f3;
  border-radius: 10px;
}

.type-badge {
  flex-shrink: 0;
  width: 44px;
  height: 24px;
  border-radius: 6px;
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
}

.res-meta {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.res-name {
  font-size: 14px;
  color: #1f2937;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.res-sub {
  font-size: 12px;
  color: #9ca3af;
}

.res-btn {
  flex-shrink: 0;
  font-size: 13px;
  color: #2563eb;
  text-decoration: none;
  padding: 4px 10px;
  border: 1px solid #d6e0f5;
  border-radius: 6px;
  transition: background 0.15s;
}

.res-btn:hover {
  background: #eff6ff;
}

.res-btn-download {
  color: #059669;
  border-color: #d1fae5;
}

.res-btn-download:hover {
  background: #ecfdf5;
}
</style>

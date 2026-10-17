<script setup>
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { createCourse } from '../api/index.js'

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
  </div>
</template>

<style scoped>
.course-create-page {
  display: flex;
  justify-content: center;
  align-items: center;
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

.title {
  text-align: center;
  margin: 0 0 16px;
}

.tip {
  color: #999;
  font-size: 13px;
  text-align: center;
}
</style>

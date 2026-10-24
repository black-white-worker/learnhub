<script setup>
import { ref, watch, computed } from 'vue'
import { useRouter, useRoute, RouterView } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Reading, Notebook, EditPen, Collection, ArrowDown, User } from '@element-plus/icons-vue'

const router = useRouter()
const route = useRoute()

// 当前登录用户（登录成功后由 LoginView 写入 localStorage）
const username = ref(localStorage.getItem('username') || '')

// 路由切换时同步登录状态：
// 在登录页登录成功后跳回首页，右上角立即从"登录"变成"退出登录"
watch(
  () => route.fullPath,
  () => {
    username.value = localStorage.getItem('username') || ''
  }
)

// 顶部导航菜单：
// path 为空的是"建设中"占位；带 children 的是下拉菜单（课程中心）
const menus = [
  { label: '首页', icon: Reading, path: '/' },
  {
    label: '课程中心',
    icon: Notebook,
    children: [
      { label: '创建课程', path: '/course/create' },
      { label: '课程列表', path: '/courses' },
    ],
  },
  { label: '班级中心', icon: User, path: '/classes' },
  { label: '考试中心', icon: EditPen, path: '' },
  { label: '资料库', icon: Collection, path: '' },
]
const activeMenu = ref('/')

// 课程中心下拉菜单是否高亮：路由命中创建课程/课程列表任一个就高亮
const isCourseMenuActive = computed(() =>
  ['/course/create', '/courses'].includes(route.path)
)

function onMenuClick(menu) {
  if (menu.path) {
    activeMenu.value = menu.path
    router.push(menu.path)
  } else {
    ElMessage.info('「' + menu.label + '」建设中，敬请期待')
  }
}

// 课程中心下拉菜单选中：跳转到对应路由并高亮
function onMenuCommand(path) {
  activeMenu.value = path
  router.push(path)
}

function goLogin() {
  router.push('/login')
}

function logout() {
  localStorage.removeItem('token')
  localStorage.removeItem('username')
  username.value = ''
  ElMessage.success('已退出登录')
  router.push('/')
}
</script>

<template>
  <el-container class="layout">
    <!-- 顶部导航（吸顶） -->
    <el-header class="header" height="64px">
      <div class="header-inner">
        <div class="logo" @click="router.push('/')">
          <span class="logo-icon">📚</span>
          <span class="logo-text">学习平台</span>
        </div>
        <nav class="menu">
          <template v-for="m in menus" :key="m.label">
            <!-- 带 children 的菜单渲染为下拉（课程中心） -->
            <el-dropdown v-if="m.children" trigger="click" @command="onMenuCommand">
              <div class="menu-item" :class="{ active: isCourseMenuActive }">
                <el-icon><component :is="m.icon" /></el-icon>
                <span>{{ m.label }}</span>
                <el-icon class="menu-arrow"><ArrowDown /></el-icon>
              </div>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item
                    v-for="c in m.children"
                    :key="c.path"
                    :command="c.path"
                  >
                    {{ c.label }}
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
            <!-- 普通菜单项 -->
            <div
              v-else
              class="menu-item"
              :class="{ active: m.path === activeMenu }"
              @click="onMenuClick(m)"
            >
              <el-icon><component :is="m.icon" /></el-icon>
              <span>{{ m.label }}</span>
            </div>
          </template>
        </nav>
        <div class="user-area">
          <template v-if="username">
            <el-avatar :size="32" class="avatar">{{ username.charAt(0).toUpperCase() }}</el-avatar>
            <span class="username">{{ username }}</span>
            <el-button link type="primary" @click="logout">退出登录</el-button>
          </template>
          <template v-else>
            <el-button type="primary" @click="goLogin">登录</el-button>
          </template>
        </div>
      </div>
    </el-header>

    <!-- 内容区 -->
    <el-main class="main">
      <RouterView />
    </el-main>

    <!-- 页脚 -->
    <el-footer class="footer">
      学习平台 · 基于 Go + Vue3 构建 · 教学演示项目
    </el-footer>
  </el-container>
</template>

<style scoped>
.layout {
  min-height: 100vh;
}

/* ===== 顶部导航 ===== */
.header {
  position: sticky;
  top: 0;
  z-index: 100;
  background: #fff;
  border-bottom: 1px solid #e5e7eb;
  padding: 0;
}

.header-inner {
  max-width: 1200px;
  height: 100%;
  margin: 0 auto;
  display: flex;
  align-items: center;
  gap: 32px;
  padding: 0 16px;
}

.logo {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  user-select: none;
}

.logo-icon {
  font-size: 24px;
}

.logo-text {
  font-size: 18px;
  font-weight: 700;
  color: #1f2937;
}

.menu {
  display: flex;
  gap: 4px;
  flex: 1;
}

.menu-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 14px;
  border-radius: 8px;
  cursor: pointer;
  color: #4b5563;
  font-size: 14px;
  transition: all 0.2s;
}

.menu-item:hover {
  background: #f3f4f6;
}

.menu-arrow {
  font-size: 12px;
  margin-left: 2px;
}

.menu-item.active {
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
  font-weight: 600;
}

/* ===== 用户区 ===== */
.user-area {
  display: flex;
  align-items: center;
  gap: 10px;
}

.avatar {
  background: var(--el-color-primary);
  color: #fff;
  font-weight: 600;
}

.username {
  font-size: 14px;
  color: #374151;
}

/* ===== 内容区 / 页脚 ===== */
.main {
  padding: 0;
}

.footer {
  text-align: center;
  color: #9ca3af;
  font-size: 13px;
  background: #fff;
  border-top: 1px solid #e5e7eb;
}
</style>

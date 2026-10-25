<script setup>
import { ref, watch, computed } from 'vue'
import { useRouter, useRoute, RouterView } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Reading, Notebook, EditPen, Collection, ArrowDown, User } from '@element-plus/icons-vue'

const router = useRouter()
const route = useRoute()

// 当前登录用户（登录成功后由 LoginView 写入 localStorage）
const username = ref(localStorage.getItem('username') || '')
const userRole = ref(localStorage.getItem('role') || '')

// 路由切换时同步登录状态：
watch(
  () => route.fullPath,
  () => {
    username.value = localStorage.getItem('username') || ''
    userRole.value = localStorage.getItem('role') || ''
  }
)

// 只有老师/管理员能创建课程（问题2）
const canCreateCourse = computed(() => userRole.value === 'teacher' || userRole.value === 'admin')
// 只有管理员能看到管理后台入口
const isAdmin = computed(() => userRole.value === 'admin')

// 顶部导航菜单
const menus = computed(() => {
  const base = [
    { label: '首页', icon: Reading, path: '/' },
    {
      label: '课程中心',
      icon: Notebook,
      children: [
        ...(canCreateCourse.value ? [{ label: '创建课程', path: '/course/create' }] : []),
        { label: '课程列表', path: '/courses' },
      ],
    },
    { label: '班级中心', icon: User, path: '/classes' },
    { label: '考试中心', icon: EditPen, path: '/exams' },
    ...(isAdmin.value
      ? [{
          label: '管理后台',
          icon: Collection,
          children: [
            { label: '用户管理', path: '/admin/users' },
            { label: '课程管理', path: '/admin/courses' },
          ],
        }]
      : []),
  ]
  return base
})
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

function onUserCommand(cmd) {
  if (cmd === 'profile') {
    router.push('/profile')
  } else if (cmd === 'logout') {
    logout()
  }
}

function logout() {
  localStorage.removeItem('token')
  localStorage.removeItem('username')
  localStorage.removeItem('role')
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
            <el-dropdown trigger="click" @command="onUserCommand">
              <div class="user-trigger">
                <el-avatar :size="32" class="avatar">{{ username.charAt(0).toUpperCase() }}</el-avatar>
                <span class="username">{{ username }}</span>
              </div>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="profile">个人主页</el-dropdown-item>
                  <el-dropdown-item command="logout" divided>退出登录</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
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
  background: linear-gradient(90deg, #1e3a8a 0%, #2563eb 100%);
  padding: 0;
  box-shadow: 0 2px 8px rgba(30, 58, 138, 0.25);
}

.header-inner {
  max-width: 1280px;
  height: 100%;
  margin: 0 auto;
  display: flex;
  align-items: center;
  gap: 32px;
  padding: 0 24px;
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
  color: #fff;
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
  padding: 8px 16px;
  border-radius: 8px;
  cursor: pointer;
  color: rgba(255, 255, 255, 0.85);
  font-size: 14px;
  transition: all 0.2s;
}

.menu-item:hover {
  background: rgba(255, 255, 255, 0.15);
  color: #fff;
}

.menu-arrow {
  font-size: 12px;
  margin-left: 2px;
}

.menu-item.active {
  color: #fff;
  background: rgba(255, 255, 255, 0.2);
  font-weight: 600;
}

/* ===== 用户区 ===== */
.user-area {
  display: flex;
  align-items: center;
  gap: 10px;
}

.user-trigger {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  padding: 4px 10px;
  border-radius: 8px;
}
.user-trigger:hover {
  background: rgba(255, 255, 255, 0.15);
}

.avatar {
  background: rgba(255, 255, 255, 0.25);
  color: #fff;
  font-weight: 600;
}

.username {
  font-size: 14px;
  color: #fff;
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

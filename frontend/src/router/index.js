import { createRouter, createWebHistory } from 'vue-router'
import HomeView from '../views/HomeView.vue'
import { resolveNavigation } from './guard'

// 路由表。
//
// 相对于改造前新增/修正的三点：
//   1. meta.requiresAuth：需要登录的页面统一标注，由下面的全局守卫拦截，
//      不再依赖"页面自己发现 401 再弹错"（token 过期时体验很差）；
//   2. 逐份批阅页（/assignment/:id/grade）：SpeedGrader 式工作流；
//   3. 404 兜底：地址写错不再白屏。
const routes = [
  {
    path: '/',
    name: 'home',
    component: HomeView,
    meta: { title: '首页' },
  },
  {
    path: '/login',
    name: 'login',
    component: () => import('../views/LoginView.vue'),
    meta: { title: '登录' },
  },
  {
    path: '/register',
    name: 'register',
    component: () => import('../views/RegisterView.vue'),
    meta: { title: '注册' },
  },
  {
    path: '/course/create',
    name: 'course-create',
    component: () => import('../views/CourseCreateView.vue'),
    meta: { title: '创建课程', requiresAuth: true },
  },
  {
    path: '/courses',
    name: 'courses',
    component: () => import('../views/CourseListView.vue'),
    meta: { title: '课程列表' },
  },
  {
    path: '/course/:id/watch/:videoId',
    name: 'course-watch',
    component: () => import('../views/CourseWatchView.vue'),
    // 视频需要登录 + 选课关系，未登录先在路由层拦下，避免进去吃 401
    meta: { title: '课程学习', requiresAuth: true },
  },
  {
    path: '/course/:id/resources',
    name: 'course-resources',
    component: () => import('../views/CourseResourcesView.vue'),
    meta: { title: '课程资源', requiresAuth: true },
  },
  {
    path: '/course/:id',
    name: 'course-detail',
    component: () => import('../views/CourseDetailView.vue'),
    meta: { title: '课程详情' },
  },
  {
    path: '/classes',
    name: 'classes',
    component: () => import('../views/ClassListView.vue'),
    meta: { title: '班级中心', requiresAuth: true },
  },
  {
    path: '/class/:id',
    name: 'class-detail',
    component: () => import('../views/ClassDetailView.vue'),
    meta: { title: '班级详情', requiresAuth: true },
  },
  {
    path: '/profile',
    name: 'profile',
    component: () => import('../views/ProfileView.vue'),
    meta: { title: '个人中心', requiresAuth: true },
  },
  {
    path: '/exams',
    name: 'exams',
    component: () => import('../views/ExamListView.vue'),
    meta: { title: '考试中心', requiresAuth: true },
  },
  {
    path: '/exam/:id',
    name: 'exam-detail',
    component: () => import('../views/ExamDetailView.vue'),
    meta: { title: '考试详情', requiresAuth: true },
  },
  {
    path: '/assignment/:id',
    name: 'assignment-detail',
    component: () => import('../views/AssignmentDetailView.vue'),
    meta: { title: '作业详情', requiresAuth: true },
  },
  {
    // 逐份批阅（对标 Canvas SpeedGrader）
    path: '/assignment/:id/grade',
    name: 'assignment-grade',
    component: () => import('../views/SpeedGraderView.vue'),
    meta: { title: '逐份批阅', requiresAuth: true },
  },
  {
    path: '/admin/users',
    name: 'admin-users',
    component: () => import('../views/admin/AdminUsersView.vue'),
    meta: { title: '用户管理', requiresAuth: true },
  },
  {
    path: '/admin/courses',
    name: 'admin-courses',
    component: () => import('../views/admin/AdminCoursesView.vue'),
    meta: { title: '课程管理', requiresAuth: true },
  },
  {
    // 404 兜底：放在最后，匹配所有未命中的地址
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('../views/NotFoundView.vue'),
    meta: { title: '页面不存在' },
  },
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
  // 切页回到顶部，改造前会保留上一页的滚动位置
  scrollBehavior: () => ({ top: 0 }),
})

// 全局登录守卫：未登录访问受保护页面 → 跳登录并记住来源，
// 登录成功后可以回到原页面（LoginView 读 redirect 参数）。
// 判定逻辑抽在 guard.js 里，方便单测。
router.beforeEach((to) => {
  return resolveNavigation(to, localStorage.getItem('token'))
})

// 标题：每个页面带上名字，标签页里能分辨
router.afterEach((to) => {
  const base = '学习平台'
  document.title = to.meta?.title ? `${to.meta.title} · ${base}` : base
})

export default router

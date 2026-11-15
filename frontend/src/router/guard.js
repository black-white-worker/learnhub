// 路由守卫的纯逻辑（抽出来是为了能脱离浏览器环境做单测）。
//
// 判定规则：
//   1. 需要登录（meta.requiresAuth）但本地没有 token → 跳登录，并记住来源地址；
//   2. 已登录还访问 /login → 回首页；
//   3. 页面限定了角色（meta.roles）而当前角色不在里面 → 回首页。
//      角色只从 localStorage 读，服务端才是最终裁判（AI 出题在 Python 侧还有一道
//      X-Role 校验），这里只是不让学生点进一个必然报错的页面。
//   4. 其余放行。
export function resolveNavigation(to, token, role) {
  const requiresAuth = Boolean(to?.meta?.requiresAuth)
  const hasToken = Boolean(token)

  if (requiresAuth && !hasToken) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  if (to?.path === '/login' && hasToken) {
    return { path: '/' }
  }
  const roles = to?.meta?.roles
  if (hasToken && Array.isArray(roles) && roles.length && !roles.includes(role)) {
    return { path: '/' }
  }
  return true
}

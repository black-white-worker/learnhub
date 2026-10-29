// 路由守卫的纯逻辑（抽出来是为了能脱离浏览器环境做单测）。
//
// 判定规则：
//   1. 需要登录（meta.requiresAuth）但本地没有 token → 跳登录，并记住来源地址；
//   2. 已登录还访问 /login → 回首页；
//   3. 其余放行。
export function resolveNavigation(to, token) {
  const requiresAuth = Boolean(to?.meta?.requiresAuth)
  const hasToken = Boolean(token)

  if (requiresAuth && !hasToken) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  if (to?.path === '/login' && hasToken) {
    return { path: '/' }
  }
  return true
}

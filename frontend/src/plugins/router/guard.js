/**
 * 路由守卫决策函数
 * 根据目标路由和认证状态，决定导航行为
 *
 * @param {string} routeName - 目标路由名称
 * @param {boolean} hasToken - 是否持有有效令牌
 * @returns {{ action: 'redirect', target: string } | { action: 'allow' }}
 */
export function resolveNavigation(routeName, hasToken) {
  if (routeName !== 'login' && !hasToken) {
    return { action: 'redirect', target: 'login' }
  }
  if (routeName === 'login' && hasToken) {
    return { action: 'redirect', target: 'dashboard' }
  }

  return { action: 'allow' }
}

import { describe, it, expect } from 'vitest'
import fc from 'fast-check'
import { resolveNavigation } from './guard'

// Feature: embyforge, Property 8: 未认证路由重定向
// Validates: Requirements 2.3
// 对于任意受保护的前端路由路径，当不存在有效令牌时，路由守卫应将用户重定向到登录页面

// 受保护的路由名称（除 login 外的所有业务路由）
const protectedRouteNames = ['dashboard', 'emby-config', 'scrape-anomaly', 'duplicate-media', 'episode-mapping']

describe('Property 8: 未认证路由重定向', () => {
  it('对于任意受保护路由，无 token 时应重定向到登录页', () => {
    fc.assert(
      fc.property(
        fc.constantFrom(...protectedRouteNames),
        routeName => {
          const result = resolveNavigation(routeName, false)

          expect(result.action).toBe('redirect')
          expect(result.target).toBe('login')
        },
      ),
      { numRuns: 20 },
    )
  })

  it('对于任意受保护路由，有 token 时应允许访问', () => {
    fc.assert(
      fc.property(
        fc.constantFrom(...protectedRouteNames),
        routeName => {
          const result = resolveNavigation(routeName, true)

          expect(result.action).toBe('allow')
        },
      ),
      { numRuns: 20 },
    )
  })

  it('对于登录路由，有 token 时应重定向到首页', () => {
    const result = resolveNavigation('login', true)

    expect(result.action).toBe('redirect')
    expect(result.target).toBe('dashboard')
  })

  it('对于登录路由，无 token 时应允许访问', () => {
    const result = resolveNavigation('login', false)

    expect(result.action).toBe('allow')
  })

  it('对于任意非 login 路由名称，无 token 时应重定向到登录页', () => {
    fc.assert(
      fc.property(
        fc.string({ minLength: 1 }).filter(s => s !== 'login'),
        routeName => {
          const result = resolveNavigation(routeName, false)

          expect(result.action).toBe('redirect')
          expect(result.target).toBe('login')
        },
      ),
      { numRuns: 20 },
    )
  })
})

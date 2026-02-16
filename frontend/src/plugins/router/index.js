import { createRouter, createWebHistory } from 'vue-router'
import { resolveNavigation } from './guard'
import { routes } from './routes'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
})

// 路由守卫：未认证重定向到登录页，已认证访问登录页重定向到首页
router.beforeEach((to, from, next) => {
  const token = localStorage.getItem('token')
  const result = resolveNavigation(to.name, !!token)

  if (result.action === 'redirect') {
    next({ name: result.target })
  } else {
    next()
  }
})

export default function (app) {
  app.use(router)
}
export { router }

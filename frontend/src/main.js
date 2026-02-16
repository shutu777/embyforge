import App from '@/App.vue'
import { performanceMonitor } from '@/utils/performanceMonitor'
import { resourceLoader } from '@/utils/resourceLoader'
import { registerPlugins } from '@core/utils/plugins'
import { createApp } from 'vue'

if (import.meta.env.MODE !== 'production') {
  console.log(`[${performance.now().toFixed(2)}ms] [main.js] 开始执行`)
}

// 标记应用初始化开始
performance.mark('app-init-start')

// 初始化性能监控
performanceMonitor.init()

// Styles - 必须同步加载以避免 FOUC
import '@core/scss/template/index.scss'
import '@layouts/styles/index.scss'
import '@styles/styles.scss'

if (import.meta.env.MODE !== 'production') {
  console.log(`[${performance.now().toFixed(2)}ms] [main.js] 样式加载完成`)
}

// Create vue app
const app = createApp(App)

if (import.meta.env.MODE !== 'production') {
  console.log(`[${performance.now().toFixed(2)}ms] [main.js] Vue 应用创建完成`)
}

// Register plugins
registerPlugins(app)

if (import.meta.env.MODE !== 'production') {
  console.log(`[${performance.now().toFixed(2)}ms] [main.js] 插件注册完成`)
}

// Mount vue app
app.mount('#app')

if (import.meta.env.MODE !== 'production') {
  console.log(`[${performance.now().toFixed(2)}ms] [main.js] Vue 应用挂载完成`)
}

// 标记应用初始化完成
performance.mark('app-init-end')
performance.measure('app-init', 'app-init-start', 'app-init-end')

// 触发自定义事件，用于性能监控
window.dispatchEvent(new Event('vue-mounted'))

// 使用 requestIdleCallback 延迟加载非关键资源
const loadNonCriticalResources = () => {
  // 预连接到外部域名
  resourceLoader.preconnectDomains([
    'https://fonts.googleapis.com',
    'https://cdn.jsdelivr.net',
  ])

  // 延迟加载字体
  import('inter-ui/inter-latin.css').then(() => {
    if (import.meta.env.MODE !== 'production') {
      console.log(`[${performance.now().toFixed(2)}ms] [main.js] 字体加载完成`)
    }
  }).catch(err => {
    if (import.meta.env.MODE !== 'production') {
      console.warn('[main.js] 字体加载失败:', err)
    }
  })
}

// 使用 requestIdleCallback 或 setTimeout 作为降级
if ('requestIdleCallback' in window) {
  requestIdleCallback(loadNonCriticalResources, { timeout: 2000 })
}
else {
  setTimeout(loadNonCriticalResources, 1)
}

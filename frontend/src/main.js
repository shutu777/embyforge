import App from '@/App.vue'
import { resourceLoader } from '@/utils/resourceLoader'
import { registerPlugins } from '@core/utils/plugins'
import { createApp } from 'vue'

// Styles - 必须同步加载以避免 FOUC
import '@core/scss/template/index.scss'
import '@layouts/styles/index.scss'
import '@styles/styles.scss'

// Create vue app
const app = createApp(App)

// Register plugins
registerPlugins(app)

// Mount vue app
app.mount('#app')

// 触发自定义事件，用于加载动画移除
window.dispatchEvent(new Event('vue-mounted'))

// 使用 requestIdleCallback 延迟加载非关键资源
const loadNonCriticalResources = () => {
  resourceLoader.preconnectDomains([
    'https://fonts.googleapis.com',
    'https://cdn.jsdelivr.net',
  ])
  import('inter-ui/inter-latin.css').catch(() => {})
}

if ('requestIdleCallback' in window) {
  requestIdleCallback(loadNonCriticalResources, { timeout: 2000 })
}
else {
  setTimeout(loadNonCriticalResources, 1)
}

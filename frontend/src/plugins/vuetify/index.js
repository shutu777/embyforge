import { createVuetify } from 'vuetify'
import { VBtn } from 'vuetify/components/VBtn'
import defaults from './defaults'
import { icons } from './icons'
import { themes } from './theme'

// Styles - 只导入必要的样式
import '@core/scss/template/libs/vuetify/index.scss'
import 'vuetify/styles'

export default function (app) {
  if (import.meta.env.MODE !== 'production') {
    console.log(`[${performance.now().toFixed(2)}ms] [vuetify/index.js] 开始初始化 Vuetify`)
  }
  
  // 从 localStorage 读取保存的主题
  const savedTheme = localStorage.getItem('theme') || 'light' // 默认浅色主题，加载更快
  
  const vuetify = createVuetify({
    aliases: {
      IconBtn: VBtn,
    },
    defaults,
    icons,
    theme: {
      defaultTheme: savedTheme,
      themes,
    },
  })

  app.use(vuetify)
  
  if (import.meta.env.MODE !== 'production') {
    console.log(`[${performance.now().toFixed(2)}ms] [vuetify/index.js] Vuetify 初始化完成`)
  }
}

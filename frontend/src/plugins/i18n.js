import { createI18n } from 'vue-i18n'
import zhCN from '@/locales/zh-CN'
import en from '@/locales/en'

const messages = {
  'zh-CN': zhCN,
  'en': en,
}

const i18n = createI18n({
  legacy: false, // 使用 Composition API 模式
  locale: 'zh-CN', // 默认语言设置为中文
  fallbackLocale: 'en', // 回退语言
  messages,
  globalInjection: true, // 全局注入 $t 函数
})

export default function (app) {
  app.use(i18n)
}

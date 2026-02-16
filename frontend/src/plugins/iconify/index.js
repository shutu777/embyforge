// 使用 Iconify Vue 组件实现按需加载图标
// 避免加载 1.5MB 的 icons.css 文件
import { Icon } from '@iconify/vue'

export default function () {
  if (import.meta.env.MODE !== 'production') {
    console.log(`[${performance.now().toFixed(2)}ms] [iconify] 使用 Iconify Vue 按需加载图标`)
  }
  
  // 全局注册 Icon 组件
  // 这样可以按需从 CDN 加载图标,而不是预加载所有图标
  return Icon
}

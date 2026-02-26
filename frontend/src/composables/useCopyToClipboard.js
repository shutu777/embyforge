import { useClipboard } from '@vueuse/core'
import { useSnackbar } from '@/composables/useSnackbar'

export function useCopyToClipboard() {
  const { copy } = useClipboard({ legacy: true })
  const snackbar = useSnackbar()

  // 复制文本到剪贴板，成功显示成功提示，失败显示错误提示
  async function copyText(text, label) {
    try {
      await copy(text)
      snackbar.success(`已复制: ${label || text}`)
    } catch {
      snackbar.error('复制失败')
    }
  }

  return { copyText }
}

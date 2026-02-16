import { reactive, toRefs } from 'vue'

// 全局状态，所有组件共享
const state = reactive({
  show: false,
  message: '',
  color: 'success',
  timeout: 3000,
})

let timer = null

export function useSnackbar() {
  function notify(message, color = 'success', timeout = 3000) {
    // 清除上一个定时器
    if (timer) clearTimeout(timer)
    state.show = false

    // 短暂延迟确保上一个 snackbar 关闭后再显示新的
    setTimeout(() => {
      state.message = message
      state.color = color
      state.timeout = timeout
      state.show = true
    }, 100)
  }

  function success(message) {
    notify(message, 'success')
  }

  function error(message) {
    notify(message, 'error')
  }

  function info(message) {
    notify(message, 'info')
  }

  function close() {
    state.show = false
  }

  return {
    ...toRefs(state),
    notify,
    success,
    error,
    info,
    close,
  }
}

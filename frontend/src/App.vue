<script setup>
import { onMounted, onUnmounted } from 'vue'
import { useSnackbar } from '@/composables/useSnackbar'

const { show: snackbarShow, message: snackbarMessage, color: snackbarColor, timeout: snackbarTimeout, close: snackbarClose } = useSnackbar()

if (import.meta.env.MODE !== 'production') {
  console.log(`[${performance.now().toFixed(2)}ms] [App.vue] Script setup 执行`)
}

let timeoutId = null

// 移除加载动画的函数
const removeLoadingAnimation = () => {
  const loadingBg = document.getElementById('loading-bg')
  if (loadingBg) {
    if (import.meta.env.MODE !== 'production') {
      console.log(`[${performance.now().toFixed(2)}ms] [App.vue] 找到加载动画元素，开始淡出`)
    }
    loadingBg.style.transition = 'opacity 0.3s ease-out'
    loadingBg.style.opacity = '0'
    
    setTimeout(() => {
      if (import.meta.env.MODE !== 'production') {
        console.log(`[${performance.now().toFixed(2)}ms] [App.vue] 移除加载动画元素`)
      }
      loadingBg.style.display = 'none'
    }, 300)
  }
}

// 页面加载完成的处理函数
const pageLoadedHandler = () => {
  if (import.meta.env.MODE !== 'production') {
    console.log(`[${performance.now().toFixed(2)}ms] [App.vue] 收到页面加载完成事件`)
  }
  if (timeoutId) {
    clearTimeout(timeoutId)
    timeoutId = null
  }
  removeLoadingAnimation()
}

// 立即设置事件监听器（在 setup 阶段）
window.addEventListener('page-loaded', pageLoadedHandler)

// 监听页面加载完成事件
onMounted(() => {
  if (import.meta.env.MODE !== 'production') {
    console.log(`[${performance.now().toFixed(2)}ms] [App.vue] onMounted 触发`)
  }
  
  // 设置超时保护
  timeoutId = setTimeout(() => {
    if (import.meta.env.MODE !== 'production') {
      console.warn(`[${performance.now().toFixed(2)}ms] [App.vue] 页面加载超时，强制移除加载动画`)
    }
    removeLoadingAnimation()
  }, 5000) // 5 秒超时
})

onUnmounted(() => {
  window.removeEventListener('page-loaded', pageLoadedHandler)
  if (timeoutId) {
    clearTimeout(timeoutId)
  }
})
</script>

<template>
  <VApp :style="{ background: 'rgb(var(--v-theme-background))' }">
    <RouterView />

    <!-- 全局右下角通知 -->
    <VSnackbar
      v-model="snackbarShow"
      :color="snackbarColor"
      :timeout="3000"
      location="bottom end"
      rounded="lg"
      min-width="0"
      class="global-snackbar"
    >
      <div class="d-flex align-center gap-3 pa-1">
        <VIcon
          :icon="snackbarColor === 'success' ? 'ri-checkbox-circle-line' : snackbarColor === 'error' ? 'ri-close-circle-line' : 'ri-information-line'"
          size="32"
          color="white"
        />
        <span class="text-h5 font-weight-medium" style="color: white;">{{ snackbarMessage }}</span>
      </div>
    </VSnackbar>
  </VApp>
</template>

<style lang="scss">
// 确保应用始终有背景色，避免白屏
.v-application {
  background: rgb(var(--v-theme-background)) !important;
  min-height: 100vh;
}

// 确保主内容区域也有背景色
.v-main {
  background: rgb(var(--v-theme-background)) !important;
}

// 全局样式：菜单项优化
.layout-vertical-nav {
  // 添加右侧直线边框和左右边距
  border-right: 1px solid rgba(var(--v-border-color), var(--v-border-opacity)) !important;
  padding-inline-start: 1rem !important;
  padding-inline-end: 1rem !important;
  
  // 禁用菜单项的 title 属性提示
  .nav-link,
  .nav-group {
    * {
      pointer-events: auto;
    }
    
    // 移除所有可能的 tooltip
    [title] {
      pointer-events: none;
    }
  }
  
  // 菜单链接
  .nav-link {
    position: relative;
    
    // 左侧虚线指示条 - 始终显示
    &::before {
      content: '';
      position: absolute;
      left: 0;
      top: 50%;
      transform: translateY(-50%);
      width: 3px;
      height: 0%;
      background: rgba(var(--v-theme-primary), 0.3);
      border-radius: 0 4px 4px 0;
      transition: all 0.2s ease;
      z-index: 0;
    }
    
    // 悬浮和激活时加强虚线
    &:hover::before {
      background: rgba(var(--v-theme-primary), 0.6);
      width: 4px;
    }
    
    // 所有链接都有圆角和边距
    :deep(a) {
      justify-content: flex-start !important;
      border-radius: 8px !important;
      margin-inline: 12px !important;
      width: calc(100% - 24px) !important;
      transition: all 0.2s ease;
      position: relative;
      z-index: 1;
    }
    
    // 选中状态样式
    :deep(.router-link-exact-active) {
      background: linear-gradient(-72.47deg, rgb(var(--v-theme-primary)) 22.16%, rgba(var(--v-theme-primary), 0.7) 76.47%) !important;
      border-radius: 8px !important;
    }
    
    // 选中状态的虚线更明显且更短
    &:has(.router-link-exact-active)::before {
      background: rgb(var(--v-theme-primary));
      width: 4px;
      height: 0%;
    }
  }
  
  // 菜单组标签
  .nav-group {
    position: relative;
    
    // 左侧虚线指示条 - 始终显示
    &::before {
      content: '';
      position: absolute;
      left: 0;
      top: 50%;
      transform: translateY(-50%);
      width: 3px;
      height: 0%;
      background: rgba(var(--v-theme-primary), 0.3);
      border-radius: 0 4px 4px 0;
      transition: all 0.2s ease;
      z-index: 0;
    }
    
    // 悬浮和打开时加强虚线
    &:hover::before,
    &.open::before {
      background: rgba(var(--v-theme-primary), 0.6);
      width: 4px;
    }
    
    :deep(.nav-group-label) {
      justify-content: flex-start !important;
      border-radius: 8px !important;
      margin-inline: 12px !important;
      width: calc(100% - 24px) !important;
      transition: all 0.2s ease;
      position: relative;
      z-index: 1;
    }
    
    // 子菜单项
    .nav-group-children {
      .nav-link {
        // 子菜单的虚线更细更淡
        &::before {
          background: rgba(var(--v-theme-primary), 0.2);
          width: 2px;
          height: 0%;
          z-index: 0;
        }
        
        &:hover::before {
          background: rgba(var(--v-theme-primary), 0.5);
          width: 3px;
        }
        
        :deep(a) {
          justify-content: flex-start !important;
          padding-inline-start: 3rem !important;
          border-radius: 8px !important;
          margin-inline: 12px !important;
          width: calc(100% - 24px) !important;
          transition: all 0.2s ease;
        }
        
        :deep(.router-link-exact-active) {
          background: linear-gradient(-72.47deg, rgb(var(--v-theme-primary)) 22.16%, rgba(var(--v-theme-primary), 0.7) 76.47%) !important;
          border-radius: 8px !important;
        }
        
        &:has(.router-link-exact-active)::before {
          background: rgb(var(--v-theme-primary));
          width: 3px;
          height: 0%;
        }
      }
    }
  }
  
  // 菜单项标题加粗
  .nav-item-title {
    font-weight: 600 !important;
  }
  
  // 隐藏菜单组的下拉箭头
  .nav-group-arrow {
    display: none !important;
  }
  
  // 隐藏滚动条（包括 perfect-scrollbar）
  .nav-items {
    scrollbar-width: none !important; // Firefox
    -ms-overflow-style: none !important; // IE and Edge
    
    &::-webkit-scrollbar {
      display: none !important; // Chrome, Safari, Opera
    }
  }
  
  // 隐藏 perfect-scrollbar 的滚动条
  .ps__rail-y,
  .ps__rail-x {
    display: none !important;
    opacity: 0 !important;
  }
  
  .ps__thumb-y,
  .ps__thumb-x {
    display: none !important;
  }
}

// 更强的选择器
.layout-nav-type-vertical .layout-vertical-nav {
  .nav-link > a,
  .nav-group > .nav-group-label {
    justify-content: flex-start !important;
  }
}

// 全局禁用导航菜单的 tooltip
.layout-vertical-nav {
  // 禁用所有 Vuetify tooltip
  .v-tooltip {
    display: none !important;
    opacity: 0 !important;
    visibility: hidden !important;
  }
  
  // 禁用所有元素的 title 属性提示
  * {
    &[title] {
      &::before,
      &::after {
        display: none !important;
      }
    }
  }
  
  // 移除所有链接和按钮的 title 属性
  a, button, span, div {
    &[title] {
      pointer-events: auto !important;
    }
  }
}

// 全局禁用所有 tooltip 在导航区域
.v-overlay-container {
  .v-tooltip {
    &:has(.layout-vertical-nav) {
      display: none !important;
    }
  }
}
</style>

<script setup>
import { defineAsyncComponent, ref, onMounted } from 'vue'

const props = defineProps({
  type: {
    type: String,
    default: 'line',
  },
  options: {
    type: Object,
    required: true,
  },
  series: {
    type: Array,
    required: true,
  },
  height: {
    type: [Number, String],
    default: 350,
  },
})

// 异步加载 VueApexCharts，避免阻塞首屏渲染
const VueApexCharts = defineAsyncComponent(() =>
  import('vue3-apexcharts').then(module => module.default),
)

const isLoaded = ref(false)
const isChartReady = ref(false)

// 延迟加载图表，让页面先渲染
onMounted(() => {
  setTimeout(() => {
    isLoaded.value = true

    // 再延迟一点，确保组件加载完成
    setTimeout(() => {
      isChartReady.value = true
    }, 100)
  }, 100)
})
</script>

<template>
  <div class="chart-wrapper">
    <!-- 加载中显示骨架屏 -->
    <div
      v-if="!isLoaded || !isChartReady"
      class="chart-skeleton"
      :style="{ height: typeof height === 'number' ? `${height}px` : height }"
    >
      <div class="skeleton-bar" />
    </div>
    
    <!-- 图表内容 - 移除 Suspense -->
    <VueApexCharts
      v-else
      :type="type"
      :options="options"
      :series="series"
      :height="height"
    />
  </div>
</template>

<style lang="scss" scoped>
.chart-wrapper {
  width: 100%;
}

.chart-skeleton {
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  
  .skeleton-bar {
    width: 80%;
    height: 60%;
    background: rgba(var(--v-theme-on-surface), 0.05);
    border-radius: 4px;
    animation: skeleton-pulse 1.5s ease-in-out infinite;
  }
}

@keyframes skeleton-pulse {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.4;
  }
}
</style>

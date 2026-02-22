<script setup>
import { ref, computed, onMounted } from 'vue'
import { useTheme } from 'vuetify'
import api from '@/utils/api'
import AsyncChart from '@/components/AsyncChart.vue'

const vuetifyTheme = useTheme()
const isDark = computed(() => vuetifyTheme.global.current.value.dark)

const loading = ref(true)
const d = ref({
  emby_connected: false,
  emby_server_name: '',
  emby_version: '',
  emby_error: '',
  movie_count: 0,
  series_count: 0,
  episode_count: 0,
  scrape_anomaly_count: 0,
  duplicate_group_count: 0,
  episode_anomaly_count: 0,
  recent_items: [],
  daily_media_stats: [],
  daily_anomaly_stats: [],
})

async function fetchDashboard() {
  loading.value = true
  try {
    const res = await api.get('/dashboard')
    d.value = res.data.data
  } catch (e) {
    console.error('获取仪表盘数据失败', e)
  } finally {
    loading.value = false
  }
}

onMounted(fetchDashboard)

const statCards = [
  { key: 'movie_count', label: '电影', icon: 'ri-movie-2-fill', color: '#6366f1' },
  { key: 'series_count', label: '剧集', icon: 'ri-tv-2-fill', color: '#06b6d4' },
  { key: 'episode_count', label: '集数', icon: 'ri-play-circle-fill', color: '#f59e0b' },
]

const anomalyCards = [
  { key: 'scrape_anomaly_count', label: '刮削异常', icon: 'ri-search-eye-fill', color: 'warning', to: '/scrape-anomaly' },
  { key: 'duplicate_group_count', label: '重复媒体', icon: 'ri-file-copy-2-fill', color: 'info', to: '/duplicate-media' },
  { key: 'episode_anomaly_count', label: '异常映射', icon: 'ri-git-branch-fill', color: 'error', to: '/episode-mapping' },
]

// 汇总计算
const totalAnomalyCount = computed(() =>
  (d.value.scrape_anomaly_count || 0) + (d.value.duplicate_group_count || 0) + (d.value.episode_anomaly_count || 0)
)
const totalMediaCount = computed(() =>
  (d.value.movie_count || 0) + (d.value.series_count || 0) + (d.value.episode_count || 0)
)

// 图表通用配置
function chartBaseOptions(categories, color, title) {
  return {
    chart: {
      type: 'area',
      toolbar: { show: false },
      sparkline: { enabled: false },
      background: 'transparent',
      fontFamily: 'inherit',
    },
    colors: [color],
    fill: {
      type: 'gradient',
      gradient: { shadeIntensity: 1, opacityFrom: 0.4, opacityTo: 0.05, stops: [0, 100] },
    },
    stroke: { curve: 'smooth', width: 2.5 },
    dataLabels: { enabled: false },
    xaxis: {
      categories,
      labels: { style: { colors: isDark.value ? '#8b949e' : '#6e7781', fontSize: '11px' } },
      axisBorder: { show: false },
      axisTicks: { show: false },
    },
    yaxis: {
      labels: { style: { colors: isDark.value ? '#8b949e' : '#6e7781', fontSize: '11px' } },
    },
    grid: {
      borderColor: isDark.value ? '#1f2b3a' : '#e5e7eb',
      strokeDashArray: 4,
      padding: { left: 8, right: 8 },
    },
    tooltip: {
      theme: isDark.value ? 'dark' : 'light',
    },
    title: {
      text: title,
      style: { fontSize: '14px', fontWeight: 600, color: isDark.value ? '#c9d1d9' : '#2e263d' },
    },
  }
}

// 入库统计图表
const mediaChartOptions = computed(() => {
  const cats = (d.value.daily_media_stats || []).map(s => s.date)
  return chartBaseOptions(cats, '#6366f1', '每日入库统计（近7天）')
})
const mediaChartSeries = computed(() => [{
  name: '入库数',
  data: (d.value.daily_media_stats || []).map(s => s.count),
}])

// 异常统计图表
const anomalyChartOptions = computed(() => {
  const cats = (d.value.daily_anomaly_stats || []).map(s => s.date)
  return chartBaseOptions(cats, '#f59e0b', '每日异常统计（近7天）')
})
const anomalyChartSeries = computed(() => [{
  name: '异常数',
  data: (d.value.daily_anomaly_stats || []).map(s => s.count),
}])
</script>

<template>
  <div class="dashboard">
    <div v-if="loading" class="d-flex justify-center align-center" style="min-height: 400px;">
      <VProgressCircular indeterminate color="primary" size="48" />
    </div>

    <template v-else>
      <!-- 第一行：统计卡片 -->
      <VRow class="mb-4">
        <VCol v-for="stat in statCards" :key="stat.key" cols="6" sm="6" md="3">
          <VCard class="dash-card" style="height: 120px;">
            <VCardText class="d-flex align-center justify-space-between h-100 pa-5 stat-card-text">
              <div>
                <div class="text-body-2 text-medium-emphasis mb-1">{{ stat.label }}</div>
                <div class="text-h4 font-weight-bold stat-number">{{ (d[stat.key] || 0).toLocaleString() }}</div>
              </div>
              <div class="stat-icon" :style="{ background: stat.color + '18' }">
                <VIcon :icon="stat.icon" :color="stat.color" size="24" />
              </div>
            </VCardText>
          </VCard>
        </VCol>
        <VCol cols="6" sm="6" md="3">
          <VCard class="dash-card" style="height: 120px;">
            <VCardText class="d-flex align-center justify-space-between h-100 pa-5 stat-card-text">
              <div>
                <div class="text-body-2 text-medium-emphasis mb-1">总异常数</div>
                <div class="text-h4 font-weight-bold stat-number">{{ totalAnomalyCount.toLocaleString() }}</div>
              </div>
              <div class="stat-icon" :style="{ background: '#ef444418' }">
                <VIcon icon="ri-alert-fill" color="#ef4444" size="24" />
              </div>
            </VCardText>
          </VCard>
        </VCol>
      </VRow>

      <!-- 第二行：最近入库 + 媒体库统计 + 异常概览 -->
      <VRow class="mb-4">
        <!-- 最近入库 -->
        <VCol cols="12" md="4">
          <VCard class="dash-card" style="height: 340px;">
            <VCardTitle class="card-title">
              <VIcon icon="ri-time-fill" color="#6366f1" size="18" class="me-2" />
              最近入库
            </VCardTitle>
            <VCardText class="pt-0 px-4 pb-3 overflow-y-auto" style="max-height: 280px;">
              <template v-if="d.recent_items && d.recent_items.length">
                <div
                  v-for="(item, i) in d.recent_items"
                  :key="item.id"
                  class="d-flex align-center py-2"
                  :class="{ 'item-border': i < d.recent_items.length - 1 }"
                >
                  <span class="text-body-2 text-medium-emphasis me-3" style="min-width: 16px; text-align: center;">{{ i + 1 }}</span>
                  <VAvatar size="36" rounded="lg" class="me-3" style="flex-shrink: 0;">
                    <VImg v-if="item.image_url" :src="item.image_url" cover />
                    <VIcon v-else icon="ri-image-line" size="18" />
                  </VAvatar>
                  <span class="text-body-2 flex-grow-1 text-truncate">{{ item.name }}</span>
                  <VChip size="x-small" :color="item.type === '电影' ? 'primary' : 'info'" variant="flat" class="ms-2">
                    {{ item.type }}
                  </VChip>
                </div>
              </template>
              <div v-else class="text-center text-body-2 text-medium-emphasis py-8">暂无数据</div>
            </VCardText>
          </VCard>
        </VCol>

        <!-- 系统状态 -->
        <VCol cols="12" md="4">
          <VCard class="dash-card" style="height: 340px;">
            <VCardTitle class="card-title">
              <VIcon icon="ri-server-fill" color="#06b6d4" size="18" class="me-2" />
              系统状态
            </VCardTitle>
            <VCardText class="d-flex flex-column justify-center px-5 pb-4 pt-0" style="height: 270px;">
              <!-- Emby 连接状态 -->
              <div class="status-row d-flex align-center py-3">
                <VAvatar color="primary" variant="tonal" size="38" rounded="lg" class="me-3">
                  <VIcon icon="ri-link" size="20" />
                </VAvatar>
                <div class="flex-grow-1">
                  <div class="text-caption text-medium-emphasis">连接状态</div>
                  <VChip :color="d.emby_connected ? 'success' : 'error'" size="x-small" variant="flat">
                    {{ d.emby_connected ? '已连接' : '未连接' }}
                  </VChip>
                </div>
              </div>
              <!-- 服务器名称 -->
              <div class="status-row d-flex align-center py-3">
                <VAvatar color="info" variant="tonal" size="38" rounded="lg" class="me-3">
                  <VIcon icon="ri-computer-fill" size="20" />
                </VAvatar>
                <div class="flex-grow-1">
                  <div class="text-caption text-medium-emphasis">服务器名称</div>
                  <div class="text-body-2 font-weight-medium">{{ d.emby_server_name || '-' }}</div>
                </div>
              </div>
              <!-- 版本号 -->
              <div class="status-row d-flex align-center py-3">
                <VAvatar color="warning" variant="tonal" size="38" rounded="lg" class="me-3">
                  <VIcon icon="ri-information-fill" size="20" />
                </VAvatar>
                <div class="flex-grow-1">
                  <div class="text-caption text-medium-emphasis">Emby 版本</div>
                  <div class="text-body-2 font-weight-medium">{{ d.emby_version || '-' }}</div>
                </div>
              </div>
              <!-- 媒体总数 -->
              <div class="d-flex align-center py-3">
                <VAvatar color="success" variant="tonal" size="38" rounded="lg" class="me-3">
                  <VIcon icon="ri-database-2-fill" size="20" />
                </VAvatar>
                <div class="flex-grow-1">
                  <div class="text-caption text-medium-emphasis">媒体总数</div>
                  <div class="text-body-2 font-weight-medium">{{ totalMediaCount.toLocaleString() }}</div>
                </div>
              </div>
            </VCardText>
          </VCard>
        </VCol>

        <!-- 异常概览 -->
        <VCol cols="12" md="4">
          <VCard class="dash-card" style="height: 340px;">
            <VCardTitle class="card-title">
              <VIcon icon="ri-alert-fill" color="#f59e0b" size="18" class="me-2" />
              异常概览
            </VCardTitle>
            <VCardText class="d-flex flex-column justify-center px-4 pb-4 pt-0" style="height: 270px;">
              <RouterLink
                v-for="item in anomalyCards"
                :key="item.key"
                :to="item.to"
                class="anomaly-row d-flex align-center py-3 text-decoration-none"
              >
                <VAvatar :color="item.color" variant="tonal" size="38" rounded="lg" class="me-3">
                  <VIcon :icon="item.icon" size="20" />
                </VAvatar>
                <span class="text-body-2 flex-grow-1" style="color: inherit;">{{ item.label }}</span>
                <span class="text-h6 font-weight-bold">{{ d[item.key] || 0 }}</span>
              </RouterLink>
            </VCardText>
          </VCard>
        </VCol>
      </VRow>

      <!-- 第三行：图表 -->
      <VRow>
        <VCol cols="12" md="6">
          <VCard class="dash-card">
            <VCardText class="pa-4">
              <AsyncChart
                type="area"
                :options="mediaChartOptions"
                :series="mediaChartSeries"
                :height="320"
              />
            </VCardText>
          </VCard>
        </VCol>
        <VCol cols="12" md="6">
          <VCard class="dash-card">
            <VCardText class="pa-4">
              <AsyncChart
                type="area"
                :options="anomalyChartOptions"
                :series="anomalyChartSeries"
                :height="320"
              />
            </VCardText>
          </VCard>
        </VCol>
      </VRow>
    </template>
  </div>
</template>

<style lang="scss" scoped>
.dash-card {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  transition: transform 0.2s ease, box-shadow 0.2s ease;

  &:hover {
    transform: translateY(-2px);
    box-shadow: 0 4px 20px rgba(0, 0, 0, 0.1);
  }
}

.card-title {
  font-size: 0.875rem !important;
  font-weight: 600;
  padding: 16px 16px 8px;
  display: flex;
  align-items: center;
}

.stat-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.item-border {
  border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}

.status-row {
  border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}

.anomaly-row {
  color: rgb(var(--v-theme-on-surface));
  border-radius: 8px;
  padding-inline: 4px;
  transition: background 0.15s ease;

  &:hover {
    background: rgba(var(--v-theme-primary), 0.06);
  }

  &:not(:last-child) {
    border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  }
}

.h-100 {
  height: 100%;
}

// 移动端响应式适配
@media (max-width: 599.98px) {
  .stat-card-text {
    padding: 12px !important;
  }

  .stat-number {
    font-size: 1.25rem !important;
  }

  .stat-icon {
    width: 40px;
    height: 40px;
  }
}

// 平板适配
@media (min-width: 600px) and (max-width: 959.98px) {
  .stat-number {
    font-size: 1.5rem !important;
  }
}
</style>

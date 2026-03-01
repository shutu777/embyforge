<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useTheme } from 'vuetify'
import api from '@/utils/api'
import AsyncChart from '@/components/AsyncChart.vue'
import { useEmbyUrl } from '@/composables/useEmbyUrl'

const vuetifyTheme = useTheme()
const isDark = computed(() => vuetifyTheme.global.current.value.dark)
const { detectEmbyUrl, embyImageUrl } = useEmbyUrl()

const loading = ref(true)
const d = ref({
  emby_connected: false,
  emby_server_name: '',
  emby_version: '',
  strm_assistant_version: '',
  emby_error: '',
  movie_count: 0,
  series_count: 0,
  episode_count: 0,
  scrape_anomaly_count: 0,
  duplicate_group_count: 0,
  episode_anomaly_count: 0,
  recent_items: [],
  recent_playback: [],
  daily_media_stats: [],
  daily_anomaly_stats: [],
})

let refreshTimer = null

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

onMounted(() => {
  fetchDashboard()
  detectEmbyUrl()
  // 每60秒自动刷新
  refreshTimer = setInterval(fetchDashboard, 60000)
})

onUnmounted(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer)
    refreshTimer = null
  }
})

const statCards = [
  { key: 'movie_count', label: '电影', icon: 'ri-movie-2-fill', color: '#6366f1' },
  { key: 'series_count', label: '剧集', icon: 'ri-tv-2-fill', color: '#06b6d4' },
  { key: 'episode_count', label: '集数', icon: 'ri-play-circle-fill', color: '#f59e0b' },
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
      <VRow class="mb-4 match-height">
        <VCol v-for="stat in statCards" :key="stat.key" cols="6" sm="6" md="3">
          <VCard class="stat-card" :style="{ borderLeft: `3px solid ${stat.color}` }">
            <VCardText class="d-flex align-center justify-space-between h-100 pa-5 stat-card-text">
              <div class="stat-text-wrap">
                <div class="text-body-2 text-medium-emphasis mb-1">{{ stat.label }}</div>
                <div class="font-weight-bold stat-number">{{ (d[stat.key] || 0).toLocaleString() }}</div>
              </div>
              <div class="stat-icon" :style="{ background: stat.color + '18' }">
                <VIcon :icon="stat.icon" :color="stat.color" size="24" />
              </div>
            </VCardText>
          </VCard>
        </VCol>
        <VCol cols="6" sm="6" md="3">
          <VCard class="stat-card" style="border-left: 3px solid #ef4444;">
            <VCardText class="d-flex align-center justify-space-between h-100 pa-5 stat-card-text">
              <div class="stat-text-wrap">
                <div class="text-body-2 text-medium-emphasis mb-1">总异常数</div>
                <div class="font-weight-bold stat-number">{{ totalAnomalyCount.toLocaleString() }}</div>
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
          <VCard class="mid-card">
            <VCardTitle class="card-title">
              <VIcon icon="ri-time-fill" color="#6366f1" size="18" class="me-2" />
              最近入库
            </VCardTitle>
            <VCardText class="pt-0 px-4 pb-3">
              <template v-if="d.recent_items && d.recent_items.length">
                <div
                  v-for="(item, i) in d.recent_items"
                  :key="item.id"
                  class="d-flex align-center py-3"
                  :class="{ 'item-border': i < d.recent_items.length - 1 }"
                >
                  <span class="text-body-2 text-medium-emphasis me-3" style="min-width: 16px; text-align: center;">{{ i + 1 }}</span>
                  <VAvatar size="36" rounded="lg" class="me-3" style="flex-shrink: 0;">
                    <VImg v-if="item.has_image" :src="embyImageUrl(item.id, 160)" cover />
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
          <VCard class="mid-card">
            <VCardTitle class="card-title">
              <VIcon icon="ri-server-fill" color="#06b6d4" size="18" class="me-2" />
              系统状态
            </VCardTitle>
            <VCardText class="d-flex flex-column flex-grow-1 px-5 pb-4 pt-0">
              <!-- Emby 连接状态 -->
              <div class="status-row d-flex align-center py-2">
                <VAvatar color="primary" variant="tonal" size="36" rounded="lg" class="me-3">
                  <VIcon icon="ri-link" size="18" />
                </VAvatar>
                <div class="flex-grow-1">
                  <div class="text-caption text-medium-emphasis">连接状态</div>
                  <VChip :color="d.emby_connected ? 'success' : 'error'" size="x-small" variant="flat">
                    {{ d.emby_connected ? '已连接' : '未连接' }}
                  </VChip>
                </div>
              </div>
              <!-- 服务器名称 -->
              <div class="status-row d-flex align-center py-2">
                <VAvatar color="info" variant="tonal" size="36" rounded="lg" class="me-3">
                  <VIcon icon="ri-computer-fill" size="18" />
                </VAvatar>
                <div class="flex-grow-1">
                  <div class="text-caption text-medium-emphasis">服务器名称</div>
                  <div class="text-body-2 font-weight-medium">{{ d.emby_server_name || '-' }}</div>
                </div>
              </div>
              <!-- 版本号 -->
              <div class="status-row d-flex align-center py-2">
                <VAvatar color="warning" variant="tonal" size="36" rounded="lg" class="me-3">
                  <VIcon icon="ri-information-fill" size="18" />
                </VAvatar>
                <div class="flex-grow-1">
                  <div class="text-caption text-medium-emphasis">Emby 版本</div>
                  <div class="text-body-2 font-weight-medium">{{ d.emby_version || '-' }}</div>
                </div>
              </div>
              <!-- Strm Assistant 版本 -->
              <div class="status-row d-flex align-center py-2">
                <VAvatar color="secondary" variant="tonal" size="36" rounded="lg" class="me-3">
                  <VIcon icon="ri-stethoscope-fill" size="18" />
                </VAvatar>
                <div class="flex-grow-1">
                  <div class="text-caption text-medium-emphasis">Strm Assistant 版本</div>
                  <div class="text-body-2 font-weight-medium">{{ d.strm_assistant_version || '-' }}</div>
                </div>
              </div>
              <!-- 媒体总数 -->
              <div class="d-flex align-center py-2">
                <VAvatar color="success" variant="tonal" size="36" rounded="lg" class="me-3">
                  <VIcon icon="ri-database-2-fill" size="18" />
                </VAvatar>
                <div class="flex-grow-1">
                  <div class="text-caption text-medium-emphasis">媒体总数</div>
                  <div class="text-body-2 font-weight-medium">{{ totalMediaCount.toLocaleString() }}</div>
                </div>
              </div>
            </VCardText>
          </VCard>
        </VCol>

        <!-- 最近播放 -->
        <VCol cols="12" md="4">
          <VCard class="mid-card">
            <VCardTitle class="card-title">
              <VIcon icon="ri-play-fill" color="#10b981" size="18" class="me-2" />
              最近播放
            </VCardTitle>
            <VCardText class="pt-0 px-4 pb-3">
              <template v-if="d.recent_playback && d.recent_playback.length">
                <div
                  v-for="(item, i) in d.recent_playback"
                  :key="i"
                  class="d-flex align-center py-2"
                  :class="{ 'item-border': i < d.recent_playback.length - 1 }"
                >
                  <VAvatar size="36" rounded="lg" class="me-3" style="flex-shrink: 0;">
                    <VImg v-if="item.item_id" :src="embyImageUrl(item.item_id, 160)" cover />
                    <VIcon v-else icon="ri-play-circle-fill" size="18" />
                  </VAvatar>
                  <div class="flex-grow-1" style="min-width: 0;">
                    <div class="text-body-2 text-truncate font-weight-medium">{{ item.media }}</div>
                    <div class="text-caption text-medium-emphasis text-truncate">{{ item.user }} · {{ item.device }} · {{ item.time }}</div>
                  </div>
                </div>
              </template>
              <div v-else class="text-center text-medium-emphasis py-8">
                <VIcon icon="ri-play-circle-line" size="32" class="mb-2 d-block mx-auto" />
                <div class="text-body-2 mb-1">暂无播放记录</div>
                <div class="text-caption">Emby 暂无播放活动，播放媒体后将自动显示</div>
              </div>
            </VCardText>
          </VCard>
        </VCol>
      </VRow>

      <!-- 第三行：图表 -->
      <VRow>
        <VCol cols="12" md="6">
          <VCard class="mid-card">
            <VCardText class="pa-4">
              <AsyncChart
                type="area"
                :options="mediaChartOptions"
                :series="mediaChartSeries"
                :height="340"
              />
            </VCardText>
          </VCard>
        </VCol>
        <VCol cols="12" md="6">
          <VCard class="mid-card">
            <VCardText class="pa-4">
              <AsyncChart
                type="area"
                :options="anomalyChartOptions"
                :series="anomalyChartSeries"
                :height="340"
              />
            </VCardText>
          </VCard>
        </VCol>
      </VRow>
    </template>
  </div>
</template>


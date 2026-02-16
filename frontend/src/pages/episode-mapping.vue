<script setup>
import { ref, computed, onMounted } from 'vue'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'

const snackbar = useSnackbar()

// 表格数据（按节目聚合）
const groups = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

// 缓存状态
const cacheStatus = ref(null)
const loadingStatus = ref(false)

// 分析状态
const analysisStatus = ref(null)
const analyzing = ref(false)
const loading = ref(false)
const analyzeResult = ref(null)

// TMDB API Key 未配置标记
const tmdbNotConfigured = ref(false)

// Emby 配置
const embyConfig = ref(null)

const hasCache = computed(() => cacheStatus.value && cacheStatus.value.total_items > 0)

const embyBaseUrl = computed(() => {
  if (!embyConfig.value) return ''
  return `${embyConfig.value.host}:${embyConfig.value.port}`
})

const embyServerId = computed(() => embyConfig.value?.server_id || '')

const lastAnalyzedAt = computed(() => {
  return analysisStatus.value?.episode_mapping?.last_analyzed_at || null
})

const anomalyCount = computed(() => {
  return analysisStatus.value?.episode_mapping?.anomaly_count || 0
})

function openInEmby(embyItemId) {
  if (!embyBaseUrl.value || !embyServerId.value) {
    snackbar.error('Emby 服务器未配置或无法获取服务器信息')
    return
  }
  const url = `${embyBaseUrl.value}/web/index.html#!/item?id=${embyItemId}&serverId=${embyServerId.value}`
  window.open(url, '_blank')
}

function openInTmdb(tmdbId) {
  window.open(`https://www.themoviedb.org/tv/${tmdbId}`, '_blank')
}

async function fetchEmbyConfig() {
  try {
    const { data } = await api.get('/emby-config/server-info')
    embyConfig.value = data.data
  } catch (e) {
    console.error('获取 Emby 服务器信息失败', e)
  }
}

function formatTime(timeStr) {
  if (!timeStr) return '-'
  const d = new Date(timeStr)
  return d.toLocaleString('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  })
}

async function fetchCacheStatus() {
  loadingStatus.value = true
  try {
    const { data } = await api.get('/cache/status')
    cacheStatus.value = data.data
  } catch (e) {
    console.error('获取缓存状态失败', e)
  } finally {
    loadingStatus.value = false
  }
}

async function fetchAnalysisStatus() {
  try {
    const { data } = await api.get('/scan/analysis-status')
    analysisStatus.value = data.data
  } catch (e) {
    console.error('获取分析状态失败', e)
  }
}

async function fetchAnomalies() {
  loading.value = true
  try {
    const { data } = await api.get('/scan/episode-mapping', {
      params: { page: page.value, pageSize: pageSize.value },
    })
    groups.value = data.data || []
    total.value = data.total || 0
  } catch (e) {
    console.error('获取异常映射数据失败', e)
  } finally {
    loading.value = false
  }
}

async function startAnalyze() {
  analyzing.value = true
  analyzeResult.value = null
  tmdbNotConfigured.value = false
  try {
    const { data } = await api.post('/analyze/episode-mapping')
    analyzeResult.value = { ...data.data, anomaly_show_count: data.anomaly_show_count }
    page.value = 1
    await Promise.all([fetchAnomalies(), fetchAnalysisStatus()])
  } catch (e) {
    const msg = e.response?.data?.message || '分析失败'
    analyzeResult.value = { error: msg }
    if (e.response?.status === 400 && msg.toLowerCase().includes('tmdb')) {
      tmdbNotConfigured.value = true
    }
    snackbar.error(msg)
  } finally {
    analyzing.value = false
  }
}

function onPageChange(newPage) {
  page.value = newPage
  fetchAnomalies()
}

onMounted(async () => {
  await fetchCacheStatus()
  await Promise.all([fetchAnomalies(), fetchAnalysisStatus(), fetchEmbyConfig()])
})
</script>

<template>
  <div class="episode-mapping-page">
    <div v-if="loadingStatus" class="d-flex justify-center align-center" style="min-height: 300px;">
      <VProgressCircular indeterminate color="primary" size="48" />
    </div>

    <template v-else>
      <VAlert v-if="!hasCache" type="warning" variant="tonal" class="mb-4">
        暂无缓存数据，请先前往
        <RouterLink to="/media-scan">扫描媒体</RouterLink>
        页面同步媒体库。
      </VAlert>

      <template v-else>
        <!-- 统计卡片 -->
        <VRow class="mb-4">
          <VCol cols="12" sm="4">
            <VCard class="stat-card" style="height: 120px;">
              <VCardText class="d-flex align-center justify-space-between h-100 pa-5">
                <div>
                  <div class="text-body-2 text-medium-emphasis mb-1">异常节目数</div>
                  <div class="text-h4 font-weight-bold">{{ anomalyCount.toLocaleString() }}</div>
                </div>
                <div class="stat-icon" style="background: #8b5cf618;">
                  <VIcon icon="ri-error-warning-fill" color="#8b5cf6" size="24" />
                </div>
              </VCardText>
            </VCard>
          </VCol>
          <VCol cols="12" sm="4">
            <VCard class="stat-card" style="height: 120px;">
              <VCardText class="d-flex align-center justify-space-between h-100 pa-5">
                <div>
                  <div class="text-body-2 text-medium-emphasis mb-1">缓存条目</div>
                  <div class="text-h4 font-weight-bold">{{ cacheStatus.total_items.toLocaleString() }}</div>
                </div>
                <div class="stat-icon" style="background: #6366f118;">
                  <VIcon icon="ri-film-fill" color="#6366f1" size="24" />
                </div>
              </VCardText>
            </VCard>
          </VCol>
          <VCol cols="12" sm="4">
            <VCard class="stat-card" style="height: 120px;">
              <VCardText class="d-flex align-center justify-space-between h-100 pa-5">
                <div>
                  <div class="text-body-2 text-medium-emphasis mb-1">最后分析</div>
                  <div class="text-h6 font-weight-bold">{{ formatTime(lastAnalyzedAt) }}</div>
                </div>
                <div class="stat-icon" style="background: #f59e0b18;">
                  <VIcon icon="ri-time-fill" color="#f59e0b" size="24" />
                </div>
              </VCardText>
            </VCard>
          </VCol>
        </VRow>

        <!-- 操作区域 -->
        <VCard variant="flat" class="content-card mb-6" data-no-hover>
          <VCardText class="pa-5">
            <div class="d-flex align-center mb-4">
              <VAvatar color="primary" variant="tonal" size="42" rounded="lg" class="me-3">
                <VIcon icon="ri-git-branch-line" size="22" />
              </VAvatar>
              <div>
                <div class="text-body-1 font-weight-semibold">集数映射检测</div>
                <div class="text-body-2 text-medium-emphasis">对比本地剧集季集数据与 TMDB 数据，找出集数不一致的季</div>
              </div>
            </div>

            <VBtn color="primary" :loading="analyzing" :disabled="analyzing" @click="startAnalyze">
              <VIcon icon="ri-play-fill" class="me-1" />
              {{ analyzing ? '分析中...' : '开始分析' }}
            </VBtn>

            <VAlert v-if="tmdbNotConfigured" type="warning" variant="tonal" class="mt-4">
              TMDB API Key 未配置，请前往
              <RouterLink to="/settings">系统设置</RouterLink>
              页面设置 TMDB API Key。
            </VAlert>

            <VCard v-if="analyzeResult && !analyzeResult.error" variant="tonal" color="success" class="mt-4">
              <VCardText class="d-flex align-center pa-4">
                <VAvatar color="success" variant="tonal" size="38" rounded="lg" class="me-3">
                  <VIcon icon="ri-check-line" size="20" />
                </VAvatar>
                <div>
                  <div class="text-body-2 font-weight-semibold">分析完成</div>
                  <div class="text-caption text-medium-emphasis">
                    共分析 {{ analyzeResult.total_scanned?.toLocaleString() }} 个节目，发现 {{ analyzeResult.anomaly_show_count?.toLocaleString() }} 个异常节目
                  </div>
                </div>
              </VCardText>
            </VCard>

            <VCard v-if="analyzeResult && analyzeResult.error" variant="tonal" color="error" class="mt-4">
              <VCardText class="d-flex align-center pa-4">
                <VAvatar color="error" variant="tonal" size="38" rounded="lg" class="me-3">
                  <VIcon icon="ri-error-warning-line" size="20" />
                </VAvatar>
                <div>
                  <div class="text-body-2 font-weight-semibold">分析失败</div>
                  <div class="text-caption text-medium-emphasis">{{ analyzeResult.error }}</div>
                </div>
              </VCardText>
            </VCard>
          </VCardText>
        </VCard>
      </template>

      <!-- 数据列表 -->
      <VCard variant="flat" class="content-card" v-if="hasCache" data-no-hover>
        <VCardText class="pa-5">
          <div class="d-flex align-center mb-4">
            <VAvatar color="warning" variant="tonal" size="42" rounded="lg" class="me-3">
              <VIcon icon="ri-list-check-2" size="22" />
            </VAvatar>
            <div>
              <div class="text-body-1 font-weight-semibold">异常映射列表</div>
              <div class="text-body-2 text-medium-emphasis">共 {{ total.toLocaleString() }} 个节目存在异常</div>
            </div>
          </div>

          <div v-if="groups.length > 0">
            <VExpansionPanels variant="accordion">
              <VExpansionPanel v-for="group in groups" :key="group.emby_item_id">
                <VExpansionPanelTitle>
                  <div class="d-flex align-center gap-2 flex-wrap">
                    <VChip size="small" color="warning">{{ group.seasons.length }} 季异常</VChip>
                    <VChip size="x-small" color="info" variant="tonal">TMDB {{ group.tmdb_id }}</VChip>
                    <span class="text-body-2 font-weight-medium">{{ group.name }}</span>
                  </div>
                </VExpansionPanelTitle>
                <VExpansionPanelText>
                  <div class="d-flex justify-end mb-2 gap-2">
                    <VBtn size="small" variant="text" color="info" @click.stop="openInTmdb(group.tmdb_id)">
                      <VIcon icon="ri-movie-2-line" size="14" class="me-1" />
                      在 TMDB 中查看
                    </VBtn>
                    <VBtn size="small" variant="text" color="primary" @click.stop="openInEmby(group.emby_item_id)">
                      <VIcon icon="ri-external-link-line" size="14" class="me-1" />
                      在 Emby 中查看
                    </VBtn>
                  </div>
                  <VTable density="compact">
                    <thead>
                      <tr>
                        <th>季</th>
                        <th>本地集数</th>
                        <th>TMDB 集数</th>
                        <th>差异</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="season in group.seasons" :key="season.id">
                        <td>Season {{ season.season_number }}</td>
                        <td>{{ season.local_episodes }}</td>
                        <td>{{ season.tmdb_episodes }}</td>
                        <td>
                          <VChip
                            size="x-small"
                            :color="season.local_episodes > season.tmdb_episodes ? 'error' : 'warning'"
                          >
                            {{ season.local_episodes > season.tmdb_episodes ? '+' : '' }}{{ season.local_episodes - season.tmdb_episodes }}
                          </VChip>
                        </td>
                      </tr>
                    </tbody>
                  </VTable>
                </VExpansionPanelText>
              </VExpansionPanel>
            </VExpansionPanels>
          </div>

          <div v-else-if="!loading" class="text-center pa-4 text-body-2 text-medium-emphasis">
            暂无数据，请点击"开始分析"按钮
          </div>
          <VProgressLinear v-if="loading" indeterminate class="mt-2" />
          <div v-if="total > pageSize" class="d-flex justify-center mt-4">
            <VPagination
              v-model="page"
              :length="Math.ceil(total / pageSize)"
              @update:model-value="onPageChange"
            />
          </div>
        </VCardText>
      </VCard>
    </template>
  </div>
</template>

<style lang="scss" scoped>
.stat-card {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}

.content-card {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  transform: none !important;
  box-shadow: none !important;
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

.h-100 {
  height: 100%;
}
</style>

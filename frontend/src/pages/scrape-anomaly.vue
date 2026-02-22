<script setup>
import { ref, computed, onMounted } from 'vue'
import { useDisplay } from 'vuetify'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'

const snackbar = useSnackbar()
const { smAndDown } = useDisplay()

// 表格数据
const anomalies = ref([])
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

// 清理状态
const cleaning = ref(false)
const cleanResult = ref(null)
const showCleanDialog = ref(false)
const selectedItems = ref([])

// 批量查找封面状态
const findingPosters = ref(false)
const findPostersResult = ref(null)
const showFindPostersDialog = ref(false)
const selectedPosterItems = ref([])
const allMissingPosterItems = ref([]) // 所有缺封面条目（跨页）
const loadingMissingPosterItems = ref(false)

// 单个查找封面状态
const findingSinglePoster = ref({}) // 记录每个条目的loading状态

// Emby 配置（用于构建跳转链接）
const embyConfig = ref(null)

// 是否有缓存数据
const hasCache = computed(() => cacheStatus.value && cacheStatus.value.total_items > 0)

// Emby 基础 URL
const embyBaseUrl = computed(() => {
  if (!embyConfig.value) return ''
  return `${embyConfig.value.host}:${embyConfig.value.port}`
})

// Emby 服务器 ID
const embyServerId = computed(() => embyConfig.value?.server_id || '')

// 最后分析时间
const lastAnalyzedAt = computed(() => {
  return analysisStatus.value?.scrape_anomaly?.last_analyzed_at || null
})

// 异常数量
const anomalyCount = computed(() => {
  return analysisStatus.value?.scrape_anomaly?.anomaly_count || 0
})

// 表格列定义
const headers = [
  { title: '媒体名称', key: 'name', width: '300px' },
  { title: '类型', key: 'type', width: '100px' },
  { title: '缺失项', key: 'missing', width: '100px' },
  { title: '路径', key: 'path' },
  { title: '操作', key: 'actions', width: '220px' },
]

// 跳转到 Emby 媒体详情页
function openInEmby(item) {
  if (!embyBaseUrl.value || !embyServerId.value) {
    snackbar.error('Emby 服务器未配置或无法获取服务器信息')
    return
  }
  const url = `${embyBaseUrl.value}/web/index.html#!/item?id=${item.emby_item_id}&serverId=${embyServerId.value}`
  window.open(url, '_blank')
}

// 获取 Emby 服务器信息
async function fetchEmbyConfig() {
  try {
    const { data } = await api.get('/emby-config/server-info')
    embyConfig.value = data.data
  } catch (e) {
    console.error('获取 Emby 服务器信息失败', e)
  }
}

// 格式化时间
function formatTime(timeStr) {
  if (!timeStr) return '-'
  const d = new Date(timeStr)
  return d.toLocaleString('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  })
}

// 获取缓存状态
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

// 获取分析状态（最后分析时间）
async function fetchAnalysisStatus() {
  try {
    const { data } = await api.get('/scan/analysis-status')
    analysisStatus.value = data.data
  } catch (e) {
    console.error('获取分析状态失败', e)
  }
}

// 获取异常列表
async function fetchAnomalies() {
  loading.value = true
  try {
    const { data } = await api.get('/scan/scrape-anomaly', {
      params: { page: page.value, pageSize: pageSize.value },
    })
    anomalies.value = data.data || []
    total.value = data.total || 0
  } catch (e) {
    console.error('获取刮削异常数据失败', e)
  } finally {
    loading.value = false
  }
}

// 开始分析
async function startAnalyze() {
  analyzing.value = true
  analyzeResult.value = null
  cleanResult.value = null
  try {
    const { data } = await api.post('/analyze/scrape-anomaly')
    analyzeResult.value = data.data
    page.value = 1
    await Promise.all([fetchAnomalies(), fetchAnalysisStatus()])
  } catch (e) {
    analyzeResult.value = { error: e.response?.data?.message || '分析失败' }
    snackbar.error(e.response?.data?.message || '分析失败')
  } finally {
    analyzing.value = false
  }
}

// 分页变化
function onPageChange(newPage) {
  page.value = newPage
  fetchAnomalies()
}

// 打开批量删除对话框
function openCleanDialog() {
  showCleanDialog.value = true
  // 默认全选
  selectedItems.value = anomalies.value.map(i => i.emby_item_id)
}

// 全选/取消全选
function toggleSelectAll() {
  const allIds = anomalies.value.map(i => i.emby_item_id)
  if (selectedItems.value.length === allIds.length) {
    selectedItems.value = []
  } else {
    selectedItems.value = [...allIds]
  }
}

// 执行批量删除
async function executeCleanup() {
  if (selectedItems.value.length === 0) {
    snackbar.error('请至少选择一个要删除的条目')
    return
  }
  showCleanDialog.value = false
  cleaning.value = true
  cleanResult.value = null
  try {
    const { data } = await api.post('/cleanup/scrape-anomaly', {
      items: selectedItems.value,
    })
    cleanResult.value = data.data
    snackbar.success(`清理完成，删除 ${data.data.deleted_count} 个条目`)
    page.value = 1
    await Promise.all([fetchAnomalies(), fetchAnalysisStatus()])
  } catch (e) {
    cleanResult.value = { error: e.response?.data?.message || '清理失败' }
    snackbar.error(e.response?.data?.message || '清理失败')
  } finally {
    cleaning.value = false
  }
}

// 打开批量查找封面对话框
async function openFindPostersDialog() {
  showFindPostersDialog.value = true
  loadingMissingPosterItems.value = true
  allMissingPosterItems.value = []
  selectedPosterItems.value = []
  try {
    const { data } = await api.get('/cleanup/missing-poster-items')
    allMissingPosterItems.value = data.data || []
    // 默认全选
    selectedPosterItems.value = allMissingPosterItems.value.map(i => i.emby_item_id)
  } catch (e) {
    console.error('获取缺封面条目失败', e)
    snackbar.error('获取缺封面条目失败')
  } finally {
    loadingMissingPosterItems.value = false
  }
}

// 全选/取消全选封面查找
function toggleSelectAllPosters() {
  const posterIds = allMissingPosterItems.value.map(i => i.emby_item_id)
  if (selectedPosterItems.value.length === posterIds.length) {
    selectedPosterItems.value = []
  } else {
    selectedPosterItems.value = [...posterIds]
  }
}

// 执行批量查找封面
async function executeFindPosters() {
  if (selectedPosterItems.value.length === 0) {
    snackbar.error('请至少选择一个要处理的条目')
    return
  }
  showFindPostersDialog.value = false
  findingPosters.value = true
  findPostersResult.value = null
  try {
    const { data } = await api.post('/cleanup/batch-find-posters', {
      items: selectedPosterItems.value,
    })
    findPostersResult.value = data.data
    const { success_count, failed_count, no_image_count } = data.data
    let message = `查找完成：成功 ${success_count} 个`
    if (no_image_count > 0) message += `，无可用图片 ${no_image_count} 个`
    if (failed_count > 0) message += `，失败 ${failed_count} 个`
    snackbar.success(message)
    page.value = 1
    await Promise.all([fetchAnomalies(), fetchAnalysisStatus()])
  } catch (e) {
    findPostersResult.value = { error: e.response?.data?.message || '查找失败' }
    snackbar.error(e.response?.data?.message || '查找失败')
  } finally {
    findingPosters.value = false
  }
}

// 单个查找封面
async function findSinglePoster(item) {
  findingSinglePoster.value[item.emby_item_id] = true
  try {
    const { data } = await api.post('/cleanup/find-single-poster', {
      item_id: item.emby_item_id,
    })
    snackbar.success(`已设置封面 (来源: ${data.data.provider_name})`)
    // 刷新列表
    await fetchAnomalies()
  } catch (e) {
    if (e.response?.status === 404) {
      snackbar.error('未找到可用的封面图片')
    } else {
      snackbar.error(e.response?.data?.message || '查找封面失败')
    }
  } finally {
    delete findingSinglePoster.value[item.emby_item_id]
  }
}

onMounted(async () => {
  await fetchCacheStatus()
  await Promise.all([fetchAnomalies(), fetchAnalysisStatus(), fetchEmbyConfig()])
})
</script>

<template>
  <div class="scrape-anomaly-page">
    <!-- 加载中 -->
    <div v-if="loadingStatus" class="d-flex justify-center align-center" style="min-height: 300px;">
      <VProgressCircular indeterminate color="primary" size="48" />
    </div>

    <template v-else>
      <!-- 无缓存数据提示 -->
      <VAlert v-if="!hasCache" type="warning" variant="tonal" class="mb-4">
        暂无缓存数据，请先前往
        <RouterLink to="/media-scan">扫描媒体</RouterLink>
        页面同步媒体库。
      </VAlert>

      <template v-else>
        <!-- 统计卡片 -->
        <VRow class="mb-4">
          <VCol cols="6" sm="4">
            <VCard class="stat-card" style="height: 120px;">
              <VCardText class="d-flex align-center justify-space-between h-100 pa-5 stat-card-text">
                <div>
                  <div class="text-body-2 text-medium-emphasis mb-1">异常数量</div>
                  <div class="text-h4 font-weight-bold stat-number">
                    {{ anomalyCount.toLocaleString() }}
                  </div>
                </div>
                <div class="stat-icon" style="background: #ef444418;">
                  <VIcon icon="ri-error-warning-fill" color="#ef4444" size="24" />
                </div>
              </VCardText>
            </VCard>
          </VCol>
          <VCol cols="6" sm="4">
            <VCard class="stat-card" style="height: 120px;">
              <VCardText class="d-flex align-center justify-space-between h-100 pa-5 stat-card-text">
                <div>
                  <div class="text-body-2 text-medium-emphasis mb-1">缓存条目</div>
                  <div class="text-h4 font-weight-bold stat-number">
                    {{ cacheStatus.total_items.toLocaleString() }}
                  </div>
                </div>
                <div class="stat-icon" style="background: #6366f118;">
                  <VIcon icon="ri-film-fill" color="#6366f1" size="24" />
                </div>
              </VCardText>
            </VCard>
          </VCol>
          <VCol cols="12" sm="4">
            <VCard class="stat-card" style="height: 120px;">
              <VCardText class="d-flex align-center justify-space-between h-100 pa-5 stat-card-text">
                <div>
                  <div class="text-body-2 text-medium-emphasis mb-1">最后分析</div>
                  <div class="text-h6 font-weight-bold">
                    {{ formatTime(lastAnalyzedAt) }}
                  </div>
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
                <VIcon icon="ri-search-eye-line" size="22" />
              </VAvatar>
              <div>
                <div class="text-body-1 font-weight-semibold">刮削异常检测</div>
                <div class="text-body-2 text-medium-emphasis">
                  检测媒体库中缺少封面图片或外部 ID（TMDB/IMDB）的条目
                </div>
              </div>
            </div>

            <div class="d-flex flex-wrap gap-3 action-buttons">
              <VBtn
                color="primary"
                :loading="analyzing"
                :disabled="analyzing || cleaning || findingPosters"
                @click="startAnalyze"
              >
                <VIcon icon="ri-play-fill" class="me-1" />
                {{ analyzing ? '分析中...' : '开始分析' }}
              </VBtn>

              <VBtn
                v-if="anomalyCount > 0"
                color="success"
                variant="tonal"
                :loading="findingPosters"
                :disabled="analyzing || cleaning || findingPosters"
                @click="openFindPostersDialog"
              >
                <VIcon icon="ri-image-add-line" class="me-1" />
                {{ findingPosters ? '查找中...' : '批量查找封面' }}
              </VBtn>

              <VBtn
                v-if="anomalyCount > 0"
                color="error"
                variant="tonal"
                :loading="cleaning"
                :disabled="analyzing || cleaning || findingPosters"
                @click="openCleanDialog"
              >
                <VIcon icon="ri-delete-bin-line" class="me-1" />
                {{ cleaning ? '删除中...' : '批量删除' }}
              </VBtn>
            </div>

            <!-- 分析结果 -->
            <VCard v-if="analyzeResult && !analyzeResult.error" variant="tonal" color="success" class="mt-4">
              <VCardText class="d-flex align-center pa-4">
                <VAvatar color="success" variant="tonal" size="38" rounded="lg" class="me-3">
                  <VIcon icon="ri-check-line" size="20" />
                </VAvatar>
                <div>
                  <div class="text-body-2 font-weight-semibold">分析完成</div>
                  <div class="text-caption text-medium-emphasis">
                    共分析 {{ analyzeResult.total_scanned?.toLocaleString() }} 项，发现 {{ analyzeResult.anomaly_count?.toLocaleString() }} 个异常
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

            <!-- 清理结果 -->
            <VCard v-if="cleanResult && !cleanResult.error" variant="tonal" color="success" class="mt-4">
              <VCardText class="d-flex align-center pa-4">
                <VAvatar color="success" variant="tonal" size="38" rounded="lg" class="me-3">
                  <VIcon icon="ri-delete-bin-line" size="20" />
                </VAvatar>
                <div>
                  <div class="text-body-2 font-weight-semibold">删除完成</div>
                  <div class="text-caption text-medium-emphasis">
                    删除 {{ cleanResult.deleted_count }} 个条目
                    <template v-if="cleanResult.failed_count > 0">
                      ，{{ cleanResult.failed_count }} 个失败
                    </template>
                  </div>
                </div>
              </VCardText>
            </VCard>

            <VCard v-if="cleanResult && cleanResult.error" variant="tonal" color="error" class="mt-4">
              <VCardText class="d-flex align-center pa-4">
                <VAvatar color="error" variant="tonal" size="38" rounded="lg" class="me-3">
                  <VIcon icon="ri-error-warning-line" size="20" />
                </VAvatar>
                <div>
                  <div class="text-body-2 font-weight-semibold">删除失败</div>
                  <div class="text-caption text-medium-emphasis">{{ cleanResult.error }}</div>
                </div>
              </VCardText>
            </VCard>

            <!-- 查找封面结果 -->
            <VCard v-if="findPostersResult && !findPostersResult.error" variant="tonal" color="success" class="mt-4">
              <VCardText class="d-flex align-center pa-4">
                <VAvatar color="success" variant="tonal" size="38" rounded="lg" class="me-3">
                  <VIcon icon="ri-image-add-line" size="20" />
                </VAvatar>
                <div>
                  <div class="text-body-2 font-weight-semibold">查找封面完成</div>
                  <div class="text-caption text-medium-emphasis">
                    成功 {{ findPostersResult.success_count }} 个
                    <template v-if="findPostersResult.no_image_count > 0">
                      ，无可用图片 {{ findPostersResult.no_image_count }} 个
                    </template>
                    <template v-if="findPostersResult.failed_count > 0">
                      ，失败 {{ findPostersResult.failed_count }} 个
                    </template>
                  </div>
                </div>
              </VCardText>
            </VCard>

            <VCard v-if="findPostersResult && findPostersResult.error" variant="tonal" color="error" class="mt-4">
              <VCardText class="d-flex align-center pa-4">
                <VAvatar color="error" variant="tonal" size="38" rounded="lg" class="me-3">
                  <VIcon icon="ri-error-warning-line" size="20" />
                </VAvatar>
                <div>
                  <div class="text-body-2 font-weight-semibold">查找封面失败</div>
                  <div class="text-caption text-medium-emphasis">{{ findPostersResult.error }}</div>
                </div>
              </VCardText>
            </VCard>
          </VCardText>
        </VCard>
      </template>

      <!-- 数据表格 -->
      <VCard variant="flat" class="content-card" v-if="hasCache" data-no-hover>
        <VCardText class="pa-5">
          <div class="d-flex align-center mb-4">
            <VAvatar color="warning" variant="tonal" size="42" rounded="lg" class="me-3">
              <VIcon icon="ri-list-check-2" size="22" />
            </VAvatar>
            <div>
              <div class="text-body-1 font-weight-semibold">刮削异常列表</div>
              <div class="text-body-2 text-medium-emphasis">
                共 {{ total.toLocaleString() }} 条记录
              </div>
            </div>
          </div>

          <!-- 移动端：卡片布局 -->
          <div v-if="anomalies.length > 0 && smAndDown" class="mobile-items">
            <div v-for="item in anomalies" :key="item.id" class="mobile-item pa-3">
              <div class="d-flex align-center justify-space-between mb-2">
                <span class="text-body-2 font-weight-medium text-truncate me-2">{{ item.name }}</span>
                <VChip size="x-small" :color="item.type === 'Movie' ? 'primary' : 'info'" variant="tonal" class="flex-shrink-0">
                  {{ item.type === 'Movie' ? '电影' : '剧集' }}
                </VChip>
              </div>
              <div class="d-flex align-center gap-1 mb-2">
                <VChip v-if="item.missing_poster" size="x-small" color="error">封面</VChip>
                <VChip v-if="item.missing_provider" size="x-small" color="warning">外部ID</VChip>
              </div>
              <div class="text-caption text-medium-emphasis path-cell mb-2">{{ item.path }}</div>
              <div class="d-flex align-center justify-end gap-1">
                <VBtn
                  v-if="item.missing_poster"
                  size="x-small"
                  variant="text"
                  color="success"
                  :loading="findingSinglePoster[item.emby_item_id]"
                  @click="findSinglePoster(item)"
                >
                  <VIcon icon="ri-image-add-line" size="14" class="me-1" />
                  查找封面
                </VBtn>
                <VBtn size="x-small" variant="text" color="primary" @click="openInEmby(item)">
                  <VIcon icon="ri-external-link-line" size="14" class="me-1" />
                  Emby
                </VBtn>
              </div>
            </div>
          </div>
          <!-- 桌面端：表格布局 -->
          <div v-else-if="anomalies.length > 0" class="table-responsive">
            <VTable>
              <thead>
                <tr>
                  <th v-for="h in headers" :key="h.key" :style="h.width ? { width: h.width } : {}">
                    {{ h.title }}
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="item in anomalies" :key="item.id">
                  <td>{{ item.name }}</td>
                  <td>
                    <VChip size="small" :color="item.type === 'Movie' ? 'primary' : 'info'" variant="tonal">
                      {{ item.type === 'Movie' ? '电影' : '剧集' }}
                    </VChip>
                  </td>
                  <td>
                    <VChip v-if="item.missing_poster" size="small" color="error" class="me-1">封面</VChip>
                    <VChip v-if="item.missing_provider" size="small" color="warning">外部ID</VChip>
                  </td>
                  <td>{{ item.path }}</td>
                  <td class="actions-cell">
                    <div class="d-flex align-center gap-1">
                      <VBtn
                        v-if="item.missing_poster"
                        size="small"
                        variant="text"
                        color="success"
                        :loading="findingSinglePoster[item.emby_item_id]"
                        @click="findSinglePoster(item)"
                      >
                        <VIcon icon="ri-image-add-line" size="14" class="me-1" />
                        查找封面
                      </VBtn>
                      <VBtn
                        size="small"
                        variant="text"
                        color="primary"
                        @click="openInEmby(item)"
                      >
                        <VIcon icon="ri-external-link-line" size="14" class="me-1" />
                        Emby
                      </VBtn>
                    </div>
                  </td>
                </tr>
              </tbody>
            </VTable>
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

    <!-- 批量删除对话框 -->
    <VDialog v-model="showCleanDialog" :max-width="smAndDown ? undefined : 1600" :fullscreen="smAndDown" scrollable transition="none">
      <VCard class="clean-dialog-card" data-no-hover>
        <VCardTitle class="d-flex align-center pa-5 pb-4">
          <VAvatar color="error" variant="tonal" size="36" rounded="lg" class="me-3">
            <VIcon icon="ri-delete-bin-line" size="18" />
          </VAvatar>
          <div>
            <div class="text-body-1 font-weight-semibold">批量删除刮削异常</div>
            <div class="text-caption text-medium-emphasis">从 Emby 媒体库中删除选中的异常条目</div>
          </div>
        </VCardTitle>

        <VDivider />

        <VCardText class="pa-0" style="max-height: 80vh;">
          <template v-if="anomalies.length > 0">
            <!-- 工具栏 -->
            <div class="clean-toolbar pa-4">
              <div class="d-flex align-center justify-space-between">
                <div class="d-flex align-center">
                  <VCheckbox
                    :model-value="selectedItems.length === anomalies.length && anomalies.length > 0"
                    :indeterminate="selectedItems.length > 0 && selectedItems.length < anomalies.length"
                    label="全选当前页"
                    density="compact"
                    hide-details
                    @click="toggleSelectAll"
                  />
                </div>
                <div class="d-flex align-center gap-3">
                  <VChip size="small" variant="tonal" color="default">
                    当前页 {{ anomalies.length }} 条
                  </VChip>
                  <VChip size="small" variant="tonal" :color="selectedItems.length > 0 ? 'error' : 'default'">
                    已选 {{ selectedItems.length }} 项
                  </VChip>
                </div>
              </div>
              <div class="text-caption text-medium-emphasis mt-2">
                选中的条目将从 Emby 媒体库中永久删除，请谨慎操作
              </div>
            </div>

            <VDivider />

            <!-- 条目列表 -->
            <div class="preview-items">
              <div
                v-for="item in anomalies"
                :key="item.emby_item_id"
                class="preview-item d-flex align-center px-5 py-3"
                :class="{ 'item-selected': selectedItems.includes(item.emby_item_id) }"
              >
                <VCheckbox
                  :model-value="selectedItems.includes(item.emby_item_id)"
                  density="compact"
                  hide-details
                  class="me-3 flex-shrink-0"
                  @update:model-value="val => {
                    if (val) selectedItems.push(item.emby_item_id)
                    else selectedItems = selectedItems.filter(id => id !== item.emby_item_id)
                  }"
                />
                <VChip
                  size="x-small"
                  :color="item.type === 'Movie' ? 'primary' : 'info'"
                  variant="tonal"
                  class="me-3 flex-shrink-0"
                  style="min-width: 42px; justify-content: center;"
                >
                  {{ item.type === 'Movie' ? '电影' : '剧集' }}
                </VChip>
                <div class="flex-grow-1 overflow-hidden">
                  <div class="text-body-2">{{ item.name }}</div>
                  <div class="text-caption text-medium-emphasis">{{ item.path }}</div>
                </div>
                <div class="d-flex align-center gap-2 flex-shrink-0 ms-4">
                  <VChip v-if="item.missing_poster" size="x-small" color="error" variant="flat">缺封面</VChip>
                  <VChip v-if="item.missing_provider" size="x-small" color="warning" variant="flat">缺外部ID</VChip>
                </div>
              </div>
            </div>
          </template>

          <div v-else class="text-center pa-12 text-body-2 text-medium-emphasis">
            没有需要删除的异常条目
          </div>
        </VCardText>

        <VDivider />

        <VCardActions class="pa-4 px-5">
          <VSpacer />
          <VBtn variant="text" @click="showCleanDialog = false">取消</VBtn>
          <VBtn
            color="error"
            :disabled="selectedItems.length === 0"
            @click="executeCleanup"
          >
            <VIcon icon="ri-delete-bin-line" class="me-1" />
            确认删除 ({{ selectedItems.length }})
          </VBtn>
        </VCardActions>
      </VCard>
    </VDialog>

    <!-- 批量查找封面对话框 -->
    <VDialog v-model="showFindPostersDialog" :max-width="smAndDown ? undefined : 1600" :fullscreen="smAndDown" scrollable transition="none">
      <VCard class="clean-dialog-card" data-no-hover>
        <VCardTitle class="d-flex align-center pa-5 pb-4">
          <VAvatar color="success" variant="tonal" size="36" rounded="lg" class="me-3">
            <VIcon icon="ri-image-add-line" size="18" />
          </VAvatar>
          <div>
            <div class="text-body-1 font-weight-semibold">批量查找封面</div>
            <div class="text-caption text-medium-emphasis">自动查找并设置第一个可用的海报图片</div>
          </div>
        </VCardTitle>

        <VDivider />

        <VCardText class="pa-0" style="max-height: 80vh;">
          <!-- 加载中 -->
          <div v-if="loadingMissingPosterItems" class="d-flex justify-center align-center pa-12">
            <VProgressCircular indeterminate color="success" size="36" class="me-3" />
            <span class="text-body-2 text-medium-emphasis">正在获取缺封面条目...</span>
          </div>

          <template v-else-if="allMissingPosterItems.length > 0">
            <!-- 工具栏 -->
            <div class="clean-toolbar pa-4">
              <div class="d-flex align-center justify-space-between">
                <div class="d-flex align-center">
                  <VCheckbox
                    :model-value="selectedPosterItems.length === allMissingPosterItems.length"
                    :indeterminate="selectedPosterItems.length > 0 && selectedPosterItems.length < allMissingPosterItems.length"
                    label="全选"
                    density="compact"
                    hide-details
                    @click="toggleSelectAllPosters"
                  />
                </div>
                <div class="d-flex align-center gap-3">
                  <VChip size="small" variant="tonal" color="default">
                    缺封面 {{ allMissingPosterItems.length }} 条
                  </VChip>
                  <VChip size="small" variant="tonal" :color="selectedPosterItems.length > 0 ? 'success' : 'default'">
                    已选 {{ selectedPosterItems.length }} 项
                  </VChip>
                </div>
              </div>
              <div class="text-caption text-medium-emphasis mt-2">
                将自动从 TMDB 等来源查找并下载第一个可用的海报图片
              </div>
            </div>

            <VDivider />

            <!-- 条目列表 -->
            <div class="preview-items">
              <div
                v-for="item in allMissingPosterItems"
                :key="item.emby_item_id"
                class="preview-item d-flex align-center px-5 py-3"
                :class="{ 'item-selected': selectedPosterItems.includes(item.emby_item_id) }"
              >
                <VCheckbox
                  :model-value="selectedPosterItems.includes(item.emby_item_id)"
                  density="compact"
                  hide-details
                  class="me-3 flex-shrink-0"
                  @update:model-value="val => {
                    if (val) selectedPosterItems.push(item.emby_item_id)
                    else selectedPosterItems = selectedPosterItems.filter(id => id !== item.emby_item_id)
                  }"
                />
                <VChip
                  size="x-small"
                  :color="item.type === 'Movie' ? 'primary' : 'info'"
                  variant="tonal"
                  class="me-3 flex-shrink-0"
                  style="min-width: 42px; justify-content: center;"
                >
                  {{ item.type === 'Movie' ? '电影' : '剧集' }}
                </VChip>
                <div class="flex-grow-1 overflow-hidden">
                  <div class="text-body-2">{{ item.name }}</div>
                  <div class="text-caption text-medium-emphasis">{{ item.path }}</div>
                </div>
                <VChip size="x-small" color="error" variant="flat" class="flex-shrink-0 ms-4">缺封面</VChip>
              </div>
            </div>
          </template>

          <div v-else class="text-center pa-12 text-body-2 text-medium-emphasis">
            没有缺少封面的条目
          </div>
        </VCardText>

        <VDivider />

        <VCardActions class="pa-4 px-5">
          <VSpacer />
          <VBtn variant="text" @click="showFindPostersDialog = false">取消</VBtn>
          <VBtn
            color="success"
            :disabled="selectedPosterItems.length === 0"
            @click="executeFindPosters"
          >
            <VIcon icon="ri-image-add-line" class="me-1" />
            开始查找 ({{ selectedPosterItems.length }})
          </VBtn>
        </VCardActions>
      </VCard>
    </VDialog>
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

.clean-dialog-card {
  border-radius: 12px !important;
}

.clean-toolbar {
  background: rgba(var(--v-theme-on-surface), 0.02);
}

.preview-items {
  .preview-item {
    border-block-end: 1px solid rgba(var(--v-border-color), 0.06);

    &.item-selected {
      background: rgba(var(--v-theme-error), 0.06);
    }

    &:last-child {
      border-block-end: none;
    }
  }
}

.table-responsive {
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
}

.mobile-items {
  .mobile-item {
    border-block-end: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));

    &:last-child {
      border-block-end: none;
    }
  }
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

  .action-buttons .v-btn {
    flex: 1 1 auto;
    min-width: 0;
  }
}

// 平板适配
@media (min-width: 600px) and (max-width: 959.98px) {
  .stat-number {
    font-size: 1.5rem !important;
  }
}
</style>

<style lang="scss">
.scrape-anomaly-page .v-table > .v-table__wrapper > table {
  table-layout: fixed;
  width: 100%;

  td, th {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  // 操作列固定在右侧
  th:last-child,
  td.actions-cell {
    position: sticky;
    right: 0;
    background: rgb(var(--v-theme-surface));
    text-align: center;
    overflow: visible;
    z-index: 1;
  }
}

@media (max-width: 599.98px) {
  .scrape-anomaly-page .v-table > .v-table__wrapper > table {
    min-width: 700px;
  }
}
</style>

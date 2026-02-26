<script setup>
import { ref, computed, onMounted, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDisplay } from 'vuetify'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'
import { useEmbyUrl } from '@/composables/useEmbyUrl'
import { useCopyToClipboard } from '@/composables/useCopyToClipboard'

const route = useRoute()
const router = useRouter()
const snackbar = useSnackbar()
const { smAndDown } = useDisplay()
const { detectEmbyUrl, embyWebUrl } = useEmbyUrl()
const { copyText } = useCopyToClipboard()

// 从 URL query 初始化状态
const page = ref(Number(route.query.page) || 1)
const pageSize = ref(20)
const searchText = ref(route.query.search || '')
const searchInput = ref(route.query.search || '')
const sortBy = ref(route.query.sort || 'season_count_desc')
const filterType = ref(route.query.filter || '')

// 表格数据
const groups = ref([])
const total = ref(0)

// 缓存状态
const cacheStatus = ref(null)
const loadingStatus = ref(false)

// 分析状态
const analysisStatus = ref(null)
const analyzing = ref(false)
const loading = ref(false)
const analyzeResult = ref(null)

// 缓存新鲜度
const cacheFreshness = ref(null)

const tmdbNotConfigured = ref(false)

// 展开面板状态
const activePanel = ref([])

// 删除相关状态
const deleteDialog = ref(false)
const deleteTarget = ref(null) // { embyItemId, name, seasonNumber? }
const deleting = ref(false)

// Emby 配置（由 useEmbyUrl 全局管理）

const hasCache = computed(() => cacheStatus.value && cacheStatus.value.total_items > 0)

const lastAnalyzedAt = computed(() => {
  return analysisStatus.value?.episode_mapping?.last_analyzed_at || null
})

const anomalyCount = computed(() => {
  return analysisStatus.value?.episode_mapping?.anomaly_count || 0
})

const sortOptions = [
  { title: '异常季数 (多→少)', value: 'season_count_desc' },
  { title: '异常季数 (少→多)', value: 'season_count_asc' },
  { title: '名称 (A→Z)', value: 'name_asc' },
  { title: '名称 (Z→A)', value: 'name_desc' },
]

// 同步状态到 URL query（带标志位防止循环触发）
let isInternalNavigation = false
function syncQueryToUrl() {
  isInternalNavigation = true
  const query = {}
  if (page.value > 1) query.page = String(page.value)
  if (searchText.value) query.search = searchText.value
  if (sortBy.value && sortBy.value !== 'season_count_desc') query.sort = sortBy.value
  if (filterType.value) query.filter = filterType.value
  router.replace({ query })
  nextTick(() => { isInternalNavigation = false })
}

function openInEmby(embyItemId) {
  const url = embyWebUrl(embyItemId)
  if (!url) {
    snackbar.error('Emby 服务器未配置或无法获取服务器信息')
    return
  }
  window.open(url, '_blank')
}

function openInTmdb(tmdbId) {
  window.open(`https://www.themoviedb.org/tv/${tmdbId}`, '_blank')
}

// Emby 地址探测（由 useEmbyUrl 全局管理）

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
    const params = {
      page: page.value,
      pageSize: pageSize.value,
      sort: sortBy.value,
    }
    if (searchText.value) params.search = searchText.value
    if (filterType.value) params.filter = filterType.value

    const { data } = await api.get('/scan/episode-mapping', { params })
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
    cacheFreshness.value = data.cache_freshness || null
    page.value = 1
    syncQueryToUrl()
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
  activePanel.value = []
  syncQueryToUrl()
  fetchAnomalies()
}

function doSearch() {
  searchText.value = searchInput.value.trim()
  page.value = 1
  activePanel.value = []
  syncQueryToUrl()
  fetchAnomalies()
}

function clearSearch() {
  searchInput.value = ''
  searchText.value = ''
  page.value = 1
  activePanel.value = []
  syncQueryToUrl()
  fetchAnomalies()
}

function setFilter(val) {
  filterType.value = val
  page.value = 1
  activePanel.value = []
  syncQueryToUrl()
  fetchAnomalies()
}

// 排序变化时重新加载
watch(sortBy, () => {
  page.value = 1
  activePanel.value = []
  syncQueryToUrl()
  fetchAnomalies()
})

// 浏览器前进/后退时从 URL 恢复状态
watch(() => route.query, () => {
  if (isInternalNavigation) return
  const newPage = Number(route.query.page) || 1
  const newSearch = route.query.search || ''
  const newSort = route.query.sort || 'season_count_desc'
  const newFilter = route.query.filter || ''

  const changed = newPage !== page.value
    || newSearch !== searchText.value
    || newSort !== sortBy.value
    || newFilter !== filterType.value

  if (!changed) return

  page.value = newPage
  searchText.value = newSearch
  searchInput.value = newSearch
  sortBy.value = newSort
  filterType.value = newFilter
  activePanel.value = []
  fetchAnomalies()
}, { deep: true })

// 删除整组
function onDeleteGroup(group) {
  deleteTarget.value = { embyItemId: group.emby_item_id, name: group.name }
  deleteDialog.value = true
}

// 删除单季
function onDeleteSeason(group, season) {
  deleteTarget.value = {
    embyItemId: group.emby_item_id,
    name: group.name,
    seasonNumber: season.season_number,
  }
  deleteDialog.value = true
}

// 确认删除
async function confirmDelete() {
  deleting.value = true
  try {
    const params = { emby_item_id: deleteTarget.value.embyItemId }
    if (deleteTarget.value.seasonNumber !== undefined) {
      params.season_number = deleteTarget.value.seasonNumber
    }
    await api.delete('/scan/episode-mapping', { data: params, timeout: 600000 })
    snackbar.success('删除成功')
    deleteDialog.value = false
    await Promise.all([fetchAnomalies(), fetchAnalysisStatus()])
  } catch (e) {
    if (e.code === 'ECONNABORTED') {
      snackbar.error('请求超时，操作可能仍在后台执行')
    } else {
      snackbar.error('删除失败: ' + (e.response?.data?.message || e.message))
    }
  } finally {
    deleting.value = false
  }
}

// 获取删除确认文本
function getDeleteConfirmText() {
  if (!deleteTarget.value) return ''
  if (deleteTarget.value.seasonNumber !== undefined) {
    return `确定要删除「${deleteTarget.value.name}」的 Season ${deleteTarget.value.seasonNumber} 异常记录吗？`
  }
  return `确定要删除「${deleteTarget.value.name}」的所有异常映射记录吗？`
}

onMounted(async () => {
  await fetchCacheStatus()
  await Promise.all([fetchAnomalies(), fetchAnalysisStatus(), detectEmbyUrl()])
})
</script>

<template>
  <div class="episode-mapping-page">
    <!-- 页面标题和说明 -->
    <div class="mb-6">
      <h1 class="text-h4 font-weight-bold mb-2">集数映射异常</h1>
      <p class="text-body-1 text-medium-emphasis">
        检测剧集中集数编号与实际文件不匹配的异常情况，帮助修正媒体库元数据
      </p>
    </div>

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
        <!-- 缓存新鲜度警告 -->
        <VAlert v-if="cacheFreshness && cacheFreshness.is_stale" type="warning" variant="tonal" class="mb-4">
          本地缓存数据可能已过期（本地 {{ cacheFreshness.local_count?.toLocaleString() }} 条，Emby {{ cacheFreshness.emby_count?.toLocaleString() }} 条），分析结果可能不准确。请先前往
          <RouterLink to="/media-scan">扫描媒体</RouterLink>
          页面执行全量同步。
        </VAlert>

        <!-- 统计卡片 -->
        <VRow class="mb-4 match-height">
          <VCol cols="6" sm="4">
            <VCard class="stat-card">
              <VCardText class="d-flex align-center justify-space-between h-100 pa-5 stat-card-text">
                <div class="stat-text-wrap">
                  <div class="text-body-2 text-medium-emphasis mb-1">异常节目数</div>
                  <div class="font-weight-bold stat-number">{{ anomalyCount.toLocaleString() }}</div>
                </div>
                <div class="stat-icon" style="background: #8b5cf618;">
                  <VIcon icon="ri-error-warning-fill" color="#8b5cf6" size="24" />
                </div>
              </VCardText>
            </VCard>
          </VCol>
          <VCol cols="6" sm="4">
            <VCard class="stat-card">
              <VCardText class="d-flex align-center justify-space-between h-100 pa-5 stat-card-text">
                <div class="stat-text-wrap">
                  <div class="text-body-2 text-medium-emphasis mb-1">缓存条目</div>
                  <div class="font-weight-bold stat-number">{{ cacheStatus.total_items.toLocaleString() }}</div>
                </div>
                <div class="stat-icon" style="background: #6366f118;">
                  <VIcon icon="ri-film-fill" color="#6366f1" size="24" />
                </div>
              </VCardText>
            </VCard>
          </VCol>
          <VCol cols="12" sm="4">
            <VCard class="stat-card">
              <VCardText class="d-flex align-center justify-space-between h-100 pa-5 stat-card-text">
                <div class="stat-text-wrap">
                  <div class="text-body-2 text-medium-emphasis mb-1">最后分析</div>
                  <div class="font-weight-bold stat-number">{{ formatTime(lastAnalyzedAt) }}</div>
                </div>
                <div class="stat-icon" style="background: #f59e0b18;">
                  <VIcon icon="ri-time-fill" color="#f59e0b" size="24" />
                </div>
              </VCardText>
            </VCard>
          </VCol>
        </VRow>

        <!-- 操作区域 -->
        <VCard variant="flat" class="content-card mb-7" data-no-hover>
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

          <!-- 搜索、排序、筛选工具栏 -->
          <VRow class="mb-4" dense>
            <VCol cols="12" sm="4">
              <VTextField
                v-model="searchInput"
                placeholder="搜索节目名称..."
                density="compact"
                variant="outlined"
                hide-details
                prepend-inner-icon="ri-search-line"
                clearable
                @keyup.enter="doSearch"
                @click:clear="clearSearch"
              />
            </VCol>
            <VCol cols="12" sm="3">
              <VSelect
                v-model="sortBy"
                :items="sortOptions"
                item-title="title"
                item-value="value"
                density="compact"
                variant="outlined"
                hide-details
                label="排序"
              />
            </VCol>
            <VCol cols="12" sm="5">
              <div class="d-flex align-center gap-2 h-100">
                <VBtn
                  :variant="filterType === '' ? 'flat' : 'outlined'"
                  :color="filterType === '' ? 'primary' : 'default'"
                  size="small"
                  @click="setFilter('')"
                >
                  全部
                </VBtn>
                <VBtn
                  :variant="filterType === 'multi' ? 'flat' : 'outlined'"
                  :color="filterType === 'multi' ? 'primary' : 'default'"
                  size="small"
                  @click="setFilter('multi')"
                >
                  多季异常
                </VBtn>
                <VBtn
                  :variant="filterType === 'single' ? 'flat' : 'outlined'"
                  :color="filterType === 'single' ? 'primary' : 'default'"
                  size="small"
                  @click="setFilter('single')"
                >
                  单季异常
                </VBtn>
              </div>
            </VCol>
          </VRow>

          <div v-if="groups.length > 0">
            <VExpansionPanels v-model="activePanel" variant="accordion">
              <VExpansionPanel v-for="group in groups" :key="group.emby_item_id">
                <VExpansionPanelTitle>
                  <div class="panel-title-content">
                    <div class="d-flex align-center gap-2">
                      <VChip size="small" color="warning">{{ group.season_count }} 季异常</VChip>
                      <VChip size="small" color="info" variant="tonal" class="copyable" @click.stop="copyText(String(group.tmdb_id), 'TMDB ' + group.tmdb_id)">TMDB {{ group.tmdb_id }}</VChip>
                    </div>
                    <div class="d-flex align-center gap-2">
                      <span class="text-body-2 font-weight-medium panel-title-name copyable" @click.stop="copyText(group.name, group.name)">{{ group.name }}</span>
                      <VBtn
                        icon
                        size="x-small"
                        variant="text"
                        color="error"
                        @click.stop="onDeleteGroup(group)"
                      >
                        <VIcon icon="ri-delete-bin-line" size="16" />
                        <VTooltip activator="parent" location="top">删除整组</VTooltip>
                      </VBtn>
                    </div>
                  </div>
                </VExpansionPanelTitle>
                <VExpansionPanelText>
                  <div class="d-flex flex-wrap justify-end mb-2 gap-2 action-links">
                    <VBtn size="small" variant="text" color="info" @click.stop="openInTmdb(group.tmdb_id)">
                      <VIcon icon="ri-movie-2-line" size="14" class="me-1" />
                      在 TMDB 中查看
                    </VBtn>
                    <VBtn size="small" variant="text" color="primary" @click.stop="openInEmby(group.emby_item_id)">
                      <VIcon icon="ri-external-link-line" size="14" class="me-1" />
                      在 Emby 中查看
                    </VBtn>
                  </div>
                  <!-- 移动端：卡片布局 -->
                  <div v-if="smAndDown" class="mobile-items">
                    <div v-for="season in group.seasons" :key="season.id" class="mobile-item pa-2">
                      <div class="d-flex align-center justify-space-between">
                        <span class="text-body-2 font-weight-medium">Season {{ season.season_number }}</span>
                        <div class="d-flex align-center gap-1">
                          <VChip
                            size="x-small"
                            :color="season.local_episodes > season.tmdb_episodes ? 'error' : 'warning'"
                          >
                            {{ season.local_episodes > season.tmdb_episodes ? '+' : '' }}{{ season.local_episodes - season.tmdb_episodes }}
                          </VChip>
                          <VBtn icon size="x-small" variant="text" color="error" @click.stop="onDeleteSeason(group, season)">
                            <VIcon icon="ri-delete-bin-line" size="14" />
                          </VBtn>
                        </div>
                      </div>
                      <div class="d-flex align-center justify-space-between text-caption text-medium-emphasis">
                        <span>本地 {{ season.local_episodes }} 集</span>
                        <span>TMDB {{ season.tmdb_episodes }} 集</span>
                      </div>
                    </div>
                  </div>
                  <!-- 桌面端：表格布局 -->
                  <VTable v-else density="compact">
                    <thead>
                      <tr>
                        <th>季</th>
                        <th>本地集数</th>
                        <th>TMDB 集数</th>
                        <th>差异</th>
                        <th style="width: 60px;">操作</th>
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
                        <td>
                          <VBtn icon size="x-small" variant="text" color="error" @click.stop="onDeleteSeason(group, season)">
                            <VIcon icon="ri-delete-bin-line" size="16" />
                            <VTooltip activator="parent" location="top">删除此季</VTooltip>
                          </VBtn>
                        </td>
                      </tr>
                    </tbody>
                  </VTable>
                </VExpansionPanelText>
              </VExpansionPanel>
            </VExpansionPanels>
          </div>

          <div v-else-if="!loading" class="text-center pa-4 text-body-2 text-medium-emphasis">
            {{ searchText || filterType ? '没有匹配的结果' : '暂无数据，请点击"开始分析"按钮' }}
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

    <!-- 删除确认对话框 -->
    <VDialog v-model="deleteDialog" max-width="400">
      <VCard data-no-hover>
        <VCardTitle class="text-body-1 font-weight-semibold pa-4">确认删除</VCardTitle>
        <VCardText class="pa-4 pt-0">
          <div class="text-body-2">{{ getDeleteConfirmText() }}</div>
          <div class="text-caption text-medium-emphasis mt-2">此操作仅删除异常映射记录，不会影响 Emby 中的实际媒体文件。</div>
        </VCardText>
        <VCardActions class="pa-4 pt-0">
          <VSpacer />
          <VBtn variant="text" @click="deleteDialog = false">取消</VBtn>
          <VBtn color="error" :loading="deleting" @click="confirmDelete">确认删除</VBtn>
        </VCardActions>
      </VCard>
    </VDialog>
  </div>
</template>

<style lang="scss" scoped>
// 页面特有样式（通用样式已提取到 page-common.scss）

@media (max-width: 599.98px) {
  .action-links {
    justify-content: flex-start !important;
    margin-bottom: 4px !important;

    .v-btn {
      font-size: 0.75rem;
      padding: 0 6px !important;
      min-width: 0;
    }
  }
}
</style>
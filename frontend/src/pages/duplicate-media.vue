<script setup>
import { ref, computed, onMounted } from 'vue'
import { useDisplay } from 'vuetify'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'

const snackbar = useSnackbar()
const { smAndDown } = useDisplay()

// 数据
const groupList = ref([])
const totalGroups = ref(0)
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
const loadingPreview = ref(false)
const previewGroups = ref([])    // 按组返回的预览数据
const selectedItems = ref([])    // 选中要删除的 emby_item_id 集合

// Emby 配置
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
  return analysisStatus.value?.duplicate_media?.last_analyzed_at || null
})

// 重复组数
const dupGroupCount = computed(() => {
  return analysisStatus.value?.duplicate_media?.anomaly_count || 0
})

// 所有可删除的条目（should_delete 标记的）
const allDeletableItems = computed(() => {
  return previewGroups.value.flatMap(g => g.items.filter(i => i.should_delete))
})

// 从 group_key 判断是电影还是剧集
function isMovieGroup(groupKey) {
  return groupKey && groupKey.startsWith('tmdb:movie:')
}

// 从 group_key 提取 TMDB ID
function extractTmdbId(groupKey) {
  if (!groupKey) return ''
  if (groupKey.startsWith('tmdb:movie:')) {
    return groupKey.substring(11)
  }
  return ''
}

// 格式化分组标题
function formatGroupTitle(group) {
  const name = group.group_name || group.group_key
  if (isMovieGroup(group.group_key)) {
    const tmdbId = extractTmdbId(group.group_key)
    return tmdbId ? `${name} {tmdb-${tmdbId}}` : name
  }
  return name
}

// 获取分组类型标签
function getGroupTypeLabel(group) {
  return isMovieGroup(group.group_key) ? '电影' : '剧集'
}

// 获取分组类型颜色
function getGroupTypeColor(group) {
  return isMovieGroup(group.group_key) ? 'primary' : 'info'
}

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

// 格式化文件大小
function formatSize(bytes) {
  if (!bytes) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let size = bytes
  while (size >= 1024 && i < units.length - 1) {
    size /= 1024
    i++
  }
  return `${size.toFixed(1)} ${units[i]}`
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

// 获取分析状态
async function fetchAnalysisStatus() {
  try {
    const { data } = await api.get('/scan/analysis-status')
    analysisStatus.value = data.data
  } catch (e) {
    console.error('获取分析状态失败', e)
  }
}

// 获取重复媒体列表
async function fetchDuplicates() {
  loading.value = true
  try {
    const { data } = await api.get('/scan/duplicate-media', {
      params: { page: page.value, pageSize: pageSize.value },
    })
    groupList.value = data.data || []
    totalGroups.value = data.total_groups || 0
  } catch (e) {
    console.error('获取重复媒体数据失败', e)
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
    const { data } = await api.post('/analyze/duplicate-media')
    analyzeResult.value = data.data
    page.value = 1
    await Promise.all([fetchDuplicates(), fetchAnalysisStatus()])
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
  fetchDuplicates()
}

// 打开清理预览对话框
async function openCleanDialog() {
  loadingPreview.value = true
  showCleanDialog.value = true
  previewGroups.value = []
  selectedItems.value = []
  try {
    const { data } = await api.get('/cleanup/duplicate-media/preview')
    previewGroups.value = data.data || []
    // 默认选中所有 should_delete 的条目
    selectedItems.value = previewGroups.value
      .flatMap(g => g.items.filter(i => i.should_delete))
      .map(i => i.emby_item_id)
  } catch (e) {
    snackbar.error('获取待清理列表失败')
    showCleanDialog.value = false
  } finally {
    loadingPreview.value = false
  }
}

// 全选/取消全选（只操作 should_delete 的条目）
function toggleSelectAll() {
  const allIds = allDeletableItems.value.map(i => i.emby_item_id)
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
    const { data } = await api.post('/cleanup/duplicate-media', {
      items: selectedItems.value,
    })
    cleanResult.value = data.data
    snackbar.success(`清理完成，删除 ${data.data.deleted_count} 个文件`)
    page.value = 1
    await Promise.all([fetchDuplicates(), fetchAnalysisStatus()])
  } catch (e) {
    cleanResult.value = { error: e.response?.data?.message || '清理失败' }
    snackbar.error(e.response?.data?.message || '清理失败')
  } finally {
    cleaning.value = false
  }
}

onMounted(async () => {
  await fetchCacheStatus()
  await Promise.all([fetchDuplicates(), fetchAnalysisStatus(), fetchEmbyConfig()])
})
</script>

<template>
  <div class="duplicate-media-page">
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
          <VCol cols="6" sm="4">
            <VCard class="stat-card stat-card-responsive">
              <VCardText class="d-flex align-center justify-space-between h-100 pa-5 stat-card-text">
                <div class="stat-text-wrap">
                  <div class="text-body-2 text-medium-emphasis mb-1">重复组数</div>
                  <div class="text-h4 font-weight-bold stat-number">
                    {{ dupGroupCount.toLocaleString() }}
                  </div>
                </div>
                <div class="stat-icon" style="background: #f9731618;">
                  <VIcon icon="ri-file-copy-2-fill" color="#f97316" size="24" />
                </div>
              </VCardText>
            </VCard>
          </VCol>
          <VCol cols="6" sm="4">
            <VCard class="stat-card stat-card-responsive">
              <VCardText class="d-flex align-center justify-space-between h-100 pa-5 stat-card-text">
                <div class="stat-text-wrap">
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
            <VCard class="stat-card stat-card-responsive">
              <VCardText class="d-flex align-center justify-space-between h-100 pa-5 stat-card-text">
                <div class="stat-text-wrap">
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
                <VIcon icon="ri-file-search-line" size="22" />
              </VAvatar>
              <div>
                <div class="text-body-1 font-weight-semibold">重复媒体检测</div>
                <div class="text-body-2 text-medium-emphasis">
                  按 TMDB ID 分组，找出媒体库中的重复条目
                </div>
              </div>
            </div>

            <div class="d-flex flex-wrap gap-3 action-buttons">
              <VBtn
                color="primary"
                :loading="analyzing"
                :disabled="analyzing || cleaning"
                @click="startAnalyze"
              >
                <VIcon icon="ri-play-fill" class="me-1" />
                {{ analyzing ? '分析中...' : '开始分析' }}
              </VBtn>

              <VBtn
                v-if="dupGroupCount > 0"
                color="error"
                variant="tonal"
                :loading="cleaning"
                :disabled="analyzing || cleaning"
                @click="openCleanDialog"
              >
                <VIcon icon="ri-delete-bin-line" class="me-1" />
                {{ cleaning ? '清理中...' : '自动清理' }}
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
                    共分析 {{ analyzeResult.total_scanned?.toLocaleString() }} 项，发现 {{ analyzeResult.anomaly_count?.toLocaleString() }} 组重复
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
                  <div class="text-body-2 font-weight-semibold">清理完成</div>
                  <div class="text-caption text-medium-emphasis">
                    删除 {{ cleanResult.deleted_count }} 个文件
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
                  <div class="text-body-2 font-weight-semibold">清理失败</div>
                  <div class="text-caption text-medium-emphasis">{{ cleanResult.error }}</div>
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
              <div class="text-body-1 font-weight-semibold">重复媒体列表</div>
              <div class="text-body-2 text-medium-emphasis">
                共 {{ totalGroups.toLocaleString() }} 组重复
              </div>
            </div>
          </div>

          <div v-if="groupList.length > 0">
            <VExpansionPanels variant="accordion">
              <VExpansionPanel v-for="group in groupList" :key="group.group_key">
                <VExpansionPanelTitle>
                  <div class="panel-title-content">
                    <div class="d-flex align-center gap-2">
                      <VChip size="small" color="warning">{{ group.count }} 个重复</VChip>
                      <VChip size="small" :color="getGroupTypeColor(group)" variant="tonal">{{ getGroupTypeLabel(group) }}</VChip>
                    </div>
                    <span class="text-body-2 font-weight-medium panel-title-name">{{ formatGroupTitle(group) }}</span>
                  </div>
                </VExpansionPanelTitle>
                <VExpansionPanelText>
                  <!-- 移动端：卡片布局 -->
                  <div v-if="smAndDown" class="mobile-items">
                    <div v-for="item in group.items" :key="item.id" class="mobile-item pa-3">
                      <div class="d-flex align-center justify-space-between mb-2">
                        <span class="text-body-2 font-weight-medium text-truncate me-2">{{ item.name }}</span>
                        <VChip size="x-small" :color="item.type === 'Movie' ? 'primary' : 'info'" variant="tonal" class="flex-shrink-0">
                          {{ item.type === 'Movie' ? '电影' : item.type === 'Series' ? '剧集' : item.type === 'Episode' ? '单集' : item.type }}
                        </VChip>
                      </div>
                      <div class="text-caption text-medium-emphasis path-cell mb-2">{{ item.path }}</div>
                      <div class="d-flex align-center justify-space-between">
                        <span class="text-caption text-medium-emphasis">{{ formatSize(item.file_size) }}</span>
                        <VBtn size="x-small" variant="text" color="primary" @click="openInEmby(item)">
                          <VIcon icon="ri-external-link-line" size="14" class="me-1" />
                          Emby
                        </VBtn>
                      </div>
                    </div>
                  </div>
                  <!-- 桌面端：表格布局 -->
                  <div v-else class="table-responsive">
                    <VTable density="compact">
                      <thead>
                        <tr>
                          <th style="width: 300px;">名称</th>
                          <th style="width: 80px;">类型</th>
                          <th>文件路径</th>
                          <th style="width: 100px;">文件大小</th>
                          <th style="width: 80px;">操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        <tr v-for="item in group.items" :key="item.id">
                          <td>{{ item.name }}</td>
                          <td>
                            <VChip size="x-small" :color="item.type === 'Movie' ? 'primary' : 'info'" variant="tonal">
                              {{ item.type === 'Movie' ? '电影' : item.type === 'Series' ? '剧集' : item.type === 'Episode' ? '单集' : item.type }}
                            </VChip>
                          </td>
                          <td class="text-body-2 path-cell">{{ item.path }}</td>
                          <td>{{ formatSize(item.file_size) }}</td>
                          <td class="text-center">
                            <VBtn
                              size="small"
                              variant="text"
                              color="primary"
                              @click="openInEmby(item)"
                            >
                              <VIcon icon="ri-external-link-line" size="14" class="me-1" />
                              Emby
                            </VBtn>
                          </td>
                        </tr>
                      </tbody>
                    </VTable>
                  </div>
                </VExpansionPanelText>
              </VExpansionPanel>
            </VExpansionPanels>
          </div>
          <div v-else-if="!loading" class="text-center pa-4 text-body-2 text-medium-emphasis">
            暂无数据，请点击"开始分析"按钮
          </div>
          <VProgressLinear v-if="loading" indeterminate class="mt-2" />
          <div v-if="totalGroups > pageSize" class="d-flex justify-center mt-4">
            <VPagination
              v-model="page"
              :length="Math.ceil(totalGroups / pageSize)"
              @update:model-value="onPageChange"
            />
          </div>
        </VCardText>
      </VCard>
    </template>

    <!-- 清理预览对话框 -->
    <VDialog v-model="showCleanDialog" :max-width="smAndDown ? undefined : 1600" :fullscreen="smAndDown" scrollable transition="none">
      <VCard class="clean-dialog-card" data-no-hover>
        <VCardTitle class="d-flex align-center pa-5 pb-4">
          <VAvatar color="error" variant="tonal" size="36" rounded="lg" class="me-3">
            <VIcon icon="ri-delete-bin-line" size="18" />
          </VAvatar>
          <div>
            <div class="text-body-1 font-weight-semibold">自动清理重复媒体</div>
            <div class="text-caption text-medium-emphasis">选择要删除的重复版本</div>
          </div>
        </VCardTitle>

        <VDivider />

        <VCardText class="pa-0" style="max-height: 80vh;">
          <!-- 加载中 -->
          <div v-if="loadingPreview" class="d-flex flex-column justify-center align-center pa-12">
            <VProgressCircular indeterminate color="primary" size="44" />
            <span class="mt-3 text-body-2 text-medium-emphasis">正在加载待清理列表...</span>
          </div>

          <template v-else-if="previewGroups.length > 0">
            <!-- 提示信息 + 全选操作栏 -->
            <div class="clean-toolbar pa-4">
              <div class="d-flex align-center justify-space-between">
                <div class="d-flex align-center">
                  <VCheckbox
                    :model-value="selectedItems.length === allDeletableItems.length && allDeletableItems.length > 0"
                    :indeterminate="selectedItems.length > 0 && selectedItems.length < allDeletableItems.length"
                    label="全选默认项"
                    density="compact"
                    hide-details
                    @click="toggleSelectAll"
                  />
                </div>
                <div class="d-flex align-center gap-3">
                  <VChip size="small" variant="tonal" color="default">
                    共 {{ previewGroups.length }} 组
                  </VChip>
                  <VChip size="small" variant="tonal" :color="selectedItems.length > 0 ? 'error' : 'default'">
                    已选 {{ selectedItems.length }} 项
                  </VChip>
                </div>
              </div>
              <div class="text-caption text-medium-emphasis mt-2">
                默认勾选体积较小的版本进行删除，你可以取消勾选或改选其他版本
              </div>
            </div>

            <VDivider />

            <!-- 按组展示 -->
            <div v-for="group in previewGroups" :key="group.group_key" class="preview-group">
              <div class="group-header px-5 py-2 d-flex align-center">
                <VChip size="x-small" :color="isMovieGroup(group.group_key) ? 'primary' : 'info'" variant="tonal" class="me-2">
                  {{ isMovieGroup(group.group_key) ? '电影' : '剧集' }}
                </VChip>
                <span class="text-body-2 font-weight-medium">{{ group.group_name }}</span>
              </div>

              <div class="preview-items">
                <div
                  v-for="item in group.items"
                  :key="item.emby_item_id"
                  class="preview-item d-flex align-center px-5 py-2"
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
                    :color="selectedItems.includes(item.emby_item_id) ? 'error' : 'success'"
                    variant="flat"
                    class="me-3 flex-shrink-0"
                    style="min-width: 42px; justify-content: center;"
                  >
                    {{ selectedItems.includes(item.emby_item_id) ? '删除' : '保留' }}
                  </VChip>
                  <div class="flex-grow-1 overflow-hidden">
                    <div class="text-body-2">{{ item.name }}</div>
                    <div class="text-caption text-medium-emphasis path-cell">{{ item.path }}</div>
                  </div>
                  <div class="text-body-2 font-weight-medium flex-shrink-0 ms-4" style="min-width: 80px; text-align: right;">
                    {{ formatSize(item.file_size) }}
                  </div>
                </div>
              </div>
            </div>
          </template>

          <div v-else class="text-center pa-12 text-body-2 text-medium-emphasis">
            没有需要清理的重复媒体
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
  </div>
</template>

<style lang="scss" scoped>
.stat-card {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}

.stat-card-responsive {
  height: 120px;
}

.stat-text-wrap {
  min-width: 0;
  flex: 1;
  overflow: hidden;
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
  margin-inline-start: 8px;
}

.h-100 {
  height: 100%;
}

.path-cell {
  word-break: break-all;
}

.clean-dialog-card {
  border-radius: 12px !important;
}

:deep(.v-dialog) {
  transition: none !important;
}

:deep(.v-overlay__content) {
  animation: none !important;
  transition: none !important;
}

.clean-toolbar {
  background: rgba(var(--v-theme-on-surface), 0.02);
}

.preview-group {
  .group-header {
    background: rgba(var(--v-theme-on-surface), 0.04);
    border-block-end: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  }
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

.panel-title-content {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 0;

  .panel-title-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

// 移动端响应式适配
@media (max-width: 599.98px) {
  .panel-title-content {
    flex-direction: column;
    align-items: flex-start;
    gap: 4px;

    .panel-title-name {
      white-space: normal;
      word-break: break-all;
    }
  }

  .stat-card-responsive {
    height: auto;
    min-height: 90px;
  }

  .stat-card-text {
    padding: 10px !important;
  }

  .stat-number {
    font-size: 1.15rem !important;
  }

  .stat-icon {
    width: 32px;
    height: 32px;
    border-radius: 8px;
    margin-inline-start: 4px;

    .v-icon {
      font-size: 16px !important;
    }
  }

  .action-buttons .v-btn {
    flex: 1 1 auto;
    min-width: 0;
  }

  .table-responsive .v-table > .v-table__wrapper > table {
    min-width: 600px;
  }
}

// 平板适配
@media (min-width: 600px) and (max-width: 959.98px) {
  .stat-number {
    font-size: 1.5rem !important;
  }
}
</style>

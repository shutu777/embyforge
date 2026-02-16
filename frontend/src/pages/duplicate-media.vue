<script setup>
import { ref, computed, onMounted } from 'vue'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'

const snackbar = useSnackbar()

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
  // 剧集分组，group_name 已经是 "剧名 S01E01" 格式
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
          <VCol cols="12" sm="4">
            <VCard class="stat-card" style="height: 120px;">
              <VCardText class="d-flex align-center justify-space-between h-100 pa-5">
                <div>
                  <div class="text-body-2 text-medium-emphasis mb-1">重复组数</div>
                  <div class="text-h4 font-weight-bold">
                    {{ dupGroupCount.toLocaleString() }}
                  </div>
                </div>
                <div class="stat-icon" style="background: #f9731618;">
                  <VIcon icon="ri-file-copy-2-fill" color="#f97316" size="24" />
                </div>
              </VCardText>
            </VCard>
          </VCol>
          <VCol cols="12" sm="4">
            <VCard class="stat-card" style="height: 120px;">
              <VCardText class="d-flex align-center justify-space-between h-100 pa-5">
                <div>
                  <div class="text-body-2 text-medium-emphasis mb-1">缓存条目</div>
                  <div class="text-h4 font-weight-bold">
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
              <VCardText class="d-flex align-center justify-space-between h-100 pa-5">
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
                <VIcon icon="ri-file-search-line" size="22" />
              </VAvatar>
              <div>
                <div class="text-body-1 font-weight-semibold">重复媒体检测</div>
                <div class="text-body-2 text-medium-emphasis">
                  按 TMDB ID 分组，找出媒体库中的重复条目
                </div>
              </div>
            </div>

            <VBtn
              color="primary"
              :loading="analyzing"
              :disabled="analyzing"
              @click="startAnalyze"
            >
              <VIcon icon="ri-play-fill" class="me-1" />
              {{ analyzing ? '分析中...' : '开始分析' }}
            </VBtn>

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
                  <div class="d-flex align-center gap-2 flex-wrap">
                    <VChip size="small" color="warning">{{ group.count }} 个重复</VChip>
                    <VChip size="x-small" :color="getGroupTypeColor(group)" variant="tonal">{{ getGroupTypeLabel(group) }}</VChip>
                    <span class="text-body-2 font-weight-medium">{{ formatGroupTitle(group) }}</span>
                  </div>
                </VExpansionPanelTitle>
                <VExpansionPanelText>
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
  </div>
</template>

<style lang="scss" scoped>
.stat-card {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  transition: transform 0.2s ease, box-shadow 0.2s ease;

  &:hover {
    transform: translateY(-2px);
    box-shadow: 0 4px 20px rgba(0, 0, 0, 0.1);
  }
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

.path-cell {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 400px;
}
</style>

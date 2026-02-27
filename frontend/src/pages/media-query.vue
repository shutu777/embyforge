<script setup>
import { ref, onMounted } from 'vue'
import { useDisplay } from 'vuetify'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'
import { useEmbyUrl } from '@/composables/useEmbyUrl'

const snackbar = useSnackbar()
const { smAndDown } = useDisplay()
const { detectEmbyUrl, embyImageUrl, embyWebUrl } = useEmbyUrl()

// 搜索相关
const searchInput = ref('')
const searchResults = ref([])
const searching = ref(false)
const searched = ref(false)

// 季选择对话框
const seasonDialog = ref(false)
const seasonLoading = ref(false)
const seasons = ref([])
const selectedSeasons = ref([])
const currentSeries = ref(null)

// 删除确认对话框
const deleteDialog = ref(false)
const deleteTarget = ref(null)
const deleteScope = ref('')
const deleting = ref(false)

// 归档对话框
const transferDialog = ref(false)
const transferItem = ref(null)
const transferMediaType = ref('movie')
const tmdbSearchInput = ref('')
const tmdbSearchResults = ref([])
const tmdbSearching = ref(false)
const selectedTmdbResult = ref(null)
const transferSeason = ref(null)
const transferring = ref(false)

function getHdhiveUrl(item) {
  if (!item?.TmdbId) return ''
  const type = item.Type === 'Movie' ? 'movie' : 'tv'
  return `https://hdhive.com/tmdb/${type}/${item.TmdbId}`
}

async function doSearch() {
  const keyword = searchInput.value.trim()
  if (!keyword) return
  searching.value = true
  searched.value = true
  try {
    const { data } = await api.get('/media-query/search', { params: { keyword } })
    searchResults.value = data.data || []
  } catch (e) {
    snackbar.error('搜索失败: ' + (e.response?.data?.message || e.message))
    searchResults.value = []
  } finally {
    searching.value = false
  }
}

function onDeleteClick(item) {
  if (item.Type === 'Movie') {
    deleteTarget.value = item
    deleteScope.value = 'movie'
    deleteDialog.value = true
  } else if (item.Type === 'Series') {
    currentSeries.value = item
    loadSeasons(item.Id)
  }
}

async function loadSeasons(seriesId) {
  seasonLoading.value = true
  seasonDialog.value = true
  selectedSeasons.value = []
  try {
    const { data } = await api.get('/media-query/seasons/' + seriesId)
    seasons.value = data.data || []
  } catch (e) {
    snackbar.error('获取季列表失败')
    seasons.value = []
  } finally {
    seasonLoading.value = false
  }
}

function deleteAllSeries() {
  seasonDialog.value = false
  deleteTarget.value = currentSeries.value
  deleteScope.value = 'series'
  deleteDialog.value = true
}

function deleteSelectedSeasons() {
  if (selectedSeasons.value.length === 0) {
    snackbar.error('请选择要删除的季')
    return
  }
  seasonDialog.value = false
  deleteTarget.value = currentSeries.value
  deleteScope.value = 'season'
  deleteDialog.value = true
}

async function confirmDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  try {
    const body = {
      emby_item_id: deleteTarget.value.Id,
      type: deleteScope.value,
      season_ids: deleteScope.value === 'season' ? selectedSeasons.value : [],
      seasons: deleteScope.value === 'season'
        ? seasons.value.filter(s => selectedSeasons.value.includes(s.id)).map(s => ({ id: s.id, season_number: s.season_number }))
        : [],
    }
    const { data } = await api.post('/media-query/delete', body)
    if (data.failed && data.failed.length > 0) {
      snackbar.error(`部分删除失败（${data.failed.length} 个）`)
    } else {
      snackbar.success('删除成功')
    }
    deleteDialog.value = false
    if (deleteScope.value === 'movie' || deleteScope.value === 'series') {
      searchResults.value = searchResults.value.filter(i => i.Id !== deleteTarget.value.Id)
    }
  } catch (e) {
    snackbar.error('删除失败: ' + (e.response?.data?.message || e.message))
  } finally {
    deleting.value = false
  }
}

function getDeleteConfirmText() {
  if (!deleteTarget.value) return ''
  if (deleteScope.value === 'movie')
    return `确定要从 Emby 中删除电影「${deleteTarget.value.Name}」吗？`
  if (deleteScope.value === 'series')
    return `确定要从 Emby 中删除剧集「${deleteTarget.value.Name}」的所有季和集吗？`
  if (deleteScope.value === 'season') {
    const count = selectedSeasons.value.length
    return `确定要从 Emby 中删除「${deleteTarget.value.Name}」的 ${count} 个季吗？`
  }
  return ''
}

function formatType(type) {
  return type === 'Movie' ? '电影' : type === 'Series' ? '剧集' : type
}

function getChildCount(item) {
  return item.ChildCount || item.RecursiveItemCount || 0
}

// 归档相关方法
function getTransferPath(item) {
  // 提取源文件夹路径：Symedia 需要的是文件夹的完整路径
  // 电影: /CloudNAS/CloudDrive/115open/影库/电影/外语电影/乱世佳人 (1939) {tmdb-770}/乱世佳人.mkv → /CloudNAS/CloudDrive/115open/影库/电影/外语电影/乱世佳人 (1939) {tmdb-770}
  // 剧集: Series 的 Path 通常已经是根目录
  if (!item?.Path) return ''
  const path = item.Path
  // 如果路径最后一段包含文件扩展名（有点号），截取到父目录
  const lastSlash = path.lastIndexOf('/')
  const lastBackslash = path.lastIndexOf('\\')
  const lastSep = Math.max(lastSlash, lastBackslash)
  if (lastSep > 0) {
    const lastPart = path.substring(lastSep + 1)
    if (lastPart.includes('.')) {
      return path.substring(0, lastSep)
    }
  }
  return path
}

function getTransferName(item) {
  // 提取源文件夹名：Symedia 需要的是文件夹名（路径最后一段），不是 Emby 的媒体标题
  // 例如: /CloudNAS/.../乱世佳人 (1939) {tmdb-770} → 乱世佳人 (1939) {tmdb-770}
  const dirPath = getTransferPath(item)
  if (!dirPath) return item?.Name || ''
  const lastSlash = dirPath.lastIndexOf('/')
  const lastBackslash = dirPath.lastIndexOf('\\')
  const lastSep = Math.max(lastSlash, lastBackslash)
  if (lastSep >= 0) {
    return dirPath.substring(lastSep + 1)
  }
  return dirPath
}

function onTransferClick(item) {
  transferItem.value = item
  transferMediaType.value = item.Type === 'Movie' ? 'movie' : 'tv'
  tmdbSearchInput.value = item.Name || ''
  tmdbSearchResults.value = []
  selectedTmdbResult.value = null
  transferSeason.value = null
  transferring.value = false
  transferDialog.value = true
}

async function doTmdbSearch() {
  const query = tmdbSearchInput.value.trim()
  if (!query) return
  tmdbSearching.value = true
  selectedTmdbResult.value = null
  try {
    const { data } = await api.get('/tmdb/search', {
      params: { query, media_type: transferMediaType.value },
    })
    tmdbSearchResults.value = data.data || []
  } catch (e) {
    snackbar.error('TMDB 搜索失败: ' + (e.response?.data?.error || e.message))
    tmdbSearchResults.value = []
  } finally {
    tmdbSearching.value = false
  }
}

function selectTmdbResult(result) {
  selectedTmdbResult.value = result
}

function getTmdbPosterUrl(posterPath) {
  if (!posterPath) return ''
  return 'https://image.tmdb.org/t/p/w185' + posterPath
}

async function confirmTransfer() {
  if (!selectedTmdbResult.value || !transferItem.value) return
  transferring.value = true
  try {
    const body = {
      name: getTransferName(transferItem.value),
      path: getTransferPath(transferItem.value),
      tmdbid: selectedTmdbResult.value.id,
      media_type: transferMediaType.value,
      season: transferMediaType.value === 'tv' && transferSeason.value != null
        ? Number(transferSeason.value)
        : null,
    }
    const { data } = await api.post('/symedia/transfer', body)
    snackbar.success(data.message || '归档请求已提交')
    transferDialog.value = false
  } catch (e) {
    snackbar.error('归档失败: ' + (e.response?.data?.error || e.message))
  } finally {
    transferring.value = false
  }
}

onMounted(() => {
  detectEmbyUrl()
})
</script>

<template>
  <div>
    <!-- 页面标题和说明 -->
    <div class="mb-6">
      <h1 class="text-h4 font-weight-bold mb-2">媒体库查询</h1>
      <p class="text-body-1 text-medium-emphasis">
        搜索 Emby 媒体库中的电影或剧集，快速定位并删除或归档指定条目
      </p>
    </div>

    <!-- 搜索卡片 -->
    <VCard variant="flat" class="content-card mb-7" data-no-hover>
      <VCardText class="pa-5">
        <div class="d-flex align-center mb-4">
          <VAvatar color="primary" variant="tonal" size="42" rounded="lg" class="me-3">
            <VIcon icon="ri-search-line" size="22" />
          </VAvatar>
          <div>
            <div class="text-body-1 font-weight-semibold">媒体库查询</div>
            <div class="text-body-2 text-medium-emphasis">搜索 Emby 媒体库，删除或归档电影和剧集</div>
          </div>
        </div>

        <VRow dense>
          <VCol cols="12" sm="8">
            <form @submit.capture.stop.prevent="doSearch">
              <VTextField
                v-model="searchInput"
                placeholder="输入名称或 TMDB ID 搜索..."
                density="compact"
                variant="outlined"
                hide-details
                prepend-inner-icon="ri-search-line"
                clearable
                enterkeyhint="search"
                @keydown.enter.capture.stop.prevent="doSearch"
                @keyup.enter.capture.stop.prevent="doSearch"
                @click:clear="searchResults = []; searched = false"
              />
            </form>
          </VCol>
          <VCol cols="12" sm="2">
            <VBtn color="primary" block :loading="searching" @click="doSearch">
              <VIcon icon="ri-search-line" class="me-1" />
              搜索
            </VBtn>
          </VCol>
        </VRow>
      </VCardText>
    </VCard>

    <!-- 搜索结果 - 海报卡片网格 -->
    <template v-if="searched">
      <div class="text-body-2 text-medium-emphasis mb-3">
        找到 {{ searchResults.length }} 个结果
      </div>

      <VProgressLinear v-if="searching" indeterminate class="mb-3" />

      <VRow v-if="searchResults.length > 0">
        <VCol v-for="item in searchResults" :key="item.Id" cols="12" md="6" xl="4">
          <VCard class="result-card" hover>
            <!-- PC 端：左右布局 -->
            <div v-if="!smAndDown" class="d-flex">
              <!-- 左侧海报 -->
              <div class="result-poster" @click="onDeleteClick(item)">
                <VImg
                  v-if="item.HasImage"
                  :src="embyImageUrl(item.Id, 300)"
                  width="130"
                  height="195"
                  cover
                />
                <div v-else class="result-poster-empty d-flex align-center justify-center">
                  <VIcon :icon="item.Type === 'Movie' ? 'ri-film-fill' : 'ri-tv-2-fill'" size="32" color="rgba(255,255,255,0.25)" />
                </div>
                <div class="result-poster-overlay">
                  <VIcon icon="ri-delete-bin-line" size="24" color="white" />
                </div>
              </div>

              <!-- 右侧信息 -->
              <div class="d-flex flex-column flex-grow-1" style="min-width: 0;">
                <VCardText class="pb-1">
                  <div class="text-h6 font-weight-bold text-truncate text-high-emphasis">{{ item.Name }}</div>
                  <div class="d-flex align-center ga-2 mt-2">
                    <VChip size="x-small" :color="item.Type === 'Movie' ? 'primary' : 'info'" variant="flat" label>
                      {{ formatType(item.Type) }}
                    </VChip>
                    <span v-if="item.ProductionYear" class="text-body-1 text-high-emphasis font-weight-medium">{{ item.ProductionYear }}</span>
                  </div>
                  <div v-if="item.Type === 'Series' && getChildCount(item)" class="text-caption text-medium-emphasis mt-1">
                    共 {{ getChildCount(item) }} 集
                  </div>
                  <!-- 额外信息 -->
                  <div class="d-flex flex-wrap ga-2 mt-3">
                    <VChip v-if="item.TmdbId" size="x-small" variant="tonal" color="success" label :href="'https://www.themoviedb.org/' + (item.Type === 'Movie' ? 'movie' : 'tv') + '/' + item.TmdbId" target="_blank" @click.stop>
                      <VIcon icon="ri-movie-2-line" size="12" class="me-1" />
                      TMDB {{ item.TmdbId }}
                    </VChip>
                    <VChip v-if="item.ImdbId" size="x-small" variant="tonal" color="warning" label :href="'https://www.imdb.com/title/' + item.ImdbId" target="_blank" @click.stop>
                      <VIcon icon="ri-star-line" size="12" class="me-1" />
                      {{ item.ImdbId }}
                    </VChip>
                    <VChip v-if="item.Type === 'Series' && getChildCount(item)" size="x-small" variant="tonal" label>
                      <VIcon icon="ri-play-list-2-line" size="12" class="me-1" />
                      {{ getChildCount(item) }} 集
                    </VChip>
                  </div>
                  <div v-if="item.Path" class="mt-2">
                    <VChip size="x-small" variant="tonal" color="secondary" label class="path-chip">
                      <VIcon icon="ri-folder-line" size="12" class="me-1" />
                      {{ item.Path }}
                    </VChip>
                  </div>
                </VCardText>

                <VSpacer />

                <VCardActions class="pt-0">
                  <VBtn
                    v-if="embyWebUrl(item.Id)"
                    :href="embyWebUrl(item.Id)"
                    target="_blank"
                    rel="noopener noreferrer"
                    variant="text"
                    color="primary"
                    size="small"
                    @click.stop
                  >
                    <VIcon icon="ri-external-link-line" size="16" class="me-1" />
                    Emby
                  </VBtn>
                  <VBtn
                    v-if="item.TmdbId"
                    :href="'https://www.themoviedb.org/' + (item.Type === 'Movie' ? 'movie' : 'tv') + '/' + item.TmdbId"
                    target="_blank"
                    rel="noopener noreferrer"
                    variant="text"
                    color="success"
                    size="small"
                    @click.stop
                  >
                    <VIcon icon="ri-movie-2-line" size="16" class="me-1" />
                    TMDB
                  </VBtn>
                  <VBtn
                    v-if="item.TmdbId"
                    :href="getHdhiveUrl(item)"
                    target="_blank"
                    rel="noopener noreferrer"
                    variant="text"
                    color="secondary"
                    size="small"
                    @click.stop
                  >
                    <VIcon icon="ri-external-link-line" size="16" class="me-1" />
                    HDHive
                  </VBtn>
                  <VBtn
                    variant="text"
                    color="warning"
                    size="small"
                    @click.stop="onTransferClick(item)"
                  >
                    <VIcon icon="ri-archive-line" size="16" class="me-1" />
                    归档
                  </VBtn>
                  <VBtn
                    variant="text"
                    color="error"
                    size="small"
                    @click.stop="onDeleteClick(item)"
                  >
                    <VIcon icon="ri-delete-bin-line" size="16" class="me-1" />
                    删除
                  </VBtn>
                </VCardActions>
              </div>
            </div>

            <!-- 移动端：上下布局 -->
            <div v-else>
              <!-- 顶部：海报 + 基本信息横排 -->
              <div class="d-flex pa-3 ga-3">
                <div class="result-poster-mobile" @click="onDeleteClick(item)">
                  <VImg
                    v-if="item.HasImage"
                    :src="embyImageUrl(item.Id, 300)"
                    width="80"
                    height="120"
                    cover
                    class="rounded"
                  />
                  <div v-else class="result-poster-empty-mobile d-flex align-center justify-center rounded">
                    <VIcon :icon="item.Type === 'Movie' ? 'ri-film-fill' : 'ri-tv-2-fill'" size="24" color="rgba(255,255,255,0.25)" />
                  </div>
                </div>
                <div class="d-flex flex-column" style="min-width: 0; flex: 1;">
                  <div class="text-body-1 font-weight-bold text-truncate text-high-emphasis">{{ item.Name }}</div>
                  <div class="d-flex align-center ga-2 mt-1">
                    <VChip size="x-small" :color="item.Type === 'Movie' ? 'primary' : 'info'" variant="flat" label>
                      {{ formatType(item.Type) }}
                    </VChip>
                    <span v-if="item.ProductionYear" class="text-body-2 text-high-emphasis">{{ item.ProductionYear }}</span>
                  </div>
                  <div class="d-flex flex-wrap ga-1 mt-2">
                    <VChip v-if="item.TmdbId" size="x-small" variant="tonal" color="success" label :href="'https://www.themoviedb.org/' + (item.Type === 'Movie' ? 'movie' : 'tv') + '/' + item.TmdbId" target="_blank" @click.stop>
                      <VIcon icon="ri-movie-2-line" size="12" class="me-1" />
                      TMDB {{ item.TmdbId }}
                    </VChip>
                    <VChip v-if="item.ImdbId" size="x-small" variant="tonal" color="warning" label :href="'https://www.imdb.com/title/' + item.ImdbId" target="_blank" @click.stop>
                      <VIcon icon="ri-star-line" size="12" class="me-1" />
                      {{ item.ImdbId }}
                    </VChip>
                  </div>
                </div>
              </div>

              <!-- 路径（独占一行，完整显示） -->
              <div v-if="item.Path" class="px-3 pb-2">
                <div class="mobile-path-box pa-2 rounded text-caption">
                  <VIcon icon="ri-folder-line" size="12" class="me-1 text-medium-emphasis flex-shrink-0" />
                  <span class="mobile-path-text">{{ item.Path }}</span>
                </div>
              </div>

              <!-- 操作按钮（独占一行，均分） -->
              <VDivider />
              <div class="mobile-action-grid">
                <div class="mobile-action-row mobile-action-row-links">
                  <VBtn
                    v-if="embyWebUrl(item.Id)"
                    :href="embyWebUrl(item.Id)"
                    target="_blank"
                    rel="noopener noreferrer"
                    variant="text"
                    color="primary"
                    size="small"
                    density="compact"
                    block
                    @click.stop
                  >
                    <VIcon icon="ri-external-link-line" size="14" class="me-1" />
                    Emby
                  </VBtn>
                  <VBtn
                    v-if="item.TmdbId"
                    :href="'https://www.themoviedb.org/' + (item.Type === 'Movie' ? 'movie' : 'tv') + '/' + item.TmdbId"
                    target="_blank"
                    rel="noopener noreferrer"
                    variant="text"
                    color="success"
                    size="small"
                    density="compact"
                    block
                    @click.stop
                  >
                    <VIcon icon="ri-movie-2-line" size="14" class="me-1" />
                    TMDB
                  </VBtn>
                  <VBtn
                    v-if="item.TmdbId"
                    :href="getHdhiveUrl(item)"
                    target="_blank"
                    rel="noopener noreferrer"
                    variant="text"
                    color="secondary"
                    size="small"
                    density="compact"
                    block
                    @click.stop
                  >
                    <VIcon icon="ri-external-link-line" size="14" class="me-1" />
                    HDHive
                  </VBtn>
                </div>

                <div class="mobile-action-row mobile-action-row-actions">
                  <VBtn
                    variant="text"
                    color="warning"
                    size="small"
                    density="compact"
                    block
                    @click.stop="onTransferClick(item)"
                  >
                    <VIcon icon="ri-archive-line" size="14" class="me-1" />
                    归档
                  </VBtn>
                  <VBtn
                    variant="text"
                    color="error"
                    size="small"
                    density="compact"
                    block
                    @click.stop="onDeleteClick(item)"
                  >
                    <VIcon icon="ri-delete-bin-line" size="14" class="me-1" />
                    删除
                  </VBtn>
                  <div class="mobile-action-spacer" />
                </div>
              </div>
            </div>
          </VCard>
        </VCol>
      </VRow>

      <div v-else-if="!searching" class="text-center pa-8 text-body-2 text-medium-emphasis">
        没有找到匹配的结果
      </div>
    </template>

    <!-- 季选择对话框 -->
    <VDialog v-model="seasonDialog" max-width="500">
      <VCard data-no-hover>
        <VCardTitle class="text-body-1 font-weight-semibold pa-4">
          选择删除范围 - {{ currentSeries?.Name }}
        </VCardTitle>
        <VCardText class="pa-4 pt-0">
          <VProgressLinear v-if="seasonLoading" indeterminate class="mb-3" />
          <template v-else>
            <div class="mb-3">
              <VBtn color="error" variant="tonal" block @click="deleteAllSeries">
                <VIcon icon="ri-delete-bin-line" class="me-1" />
                删除整个剧集（所有季）
              </VBtn>
            </div>
            <VDivider class="mb-3" />
            <div class="text-body-2 font-weight-medium mb-2">或选择要删除的季：</div>
            <div v-if="seasons.length > 0">
              <div v-for="s in seasons" :key="s.id" class="d-flex align-center py-1">
                <VCheckbox
                  v-model="selectedSeasons"
                  :value="s.id"
                  hide-details
                  density="compact"
                  class="me-2"
                />
                <span class="text-body-2">Season {{ s.season_number }}</span>
                <span class="text-caption text-medium-emphasis ms-2">{{ s.episode_count }} 集</span>
              </div>
            </div>
            <div v-else class="text-body-2 text-medium-emphasis">未找到季信息</div>
          </template>
        </VCardText>
        <VCardActions class="pa-4 pt-0">
          <VSpacer />
          <VBtn variant="text" @click="seasonDialog = false">取消</VBtn>
          <VBtn color="error" :disabled="selectedSeasons.length === 0" @click="deleteSelectedSeasons">
            删除选中的 {{ selectedSeasons.length }} 个季
          </VBtn>
        </VCardActions>
      </VCard>
    </VDialog>

    <!-- 删除确认对话框 -->
    <VDialog v-model="deleteDialog" max-width="400">
      <VCard data-no-hover>
        <VCardTitle class="text-body-1 font-weight-semibold pa-4">确认删除</VCardTitle>
        <VCardText class="pa-4 pt-0">
          <div class="text-body-2">{{ getDeleteConfirmText() }}</div>
          <div class="text-caption text-error mt-2">此操作将从 Emby 服务器永久删除文件，无法恢复。</div>
        </VCardText>
        <VCardActions class="pa-4 pt-0">
          <VSpacer />
          <VBtn variant="text" @click="deleteDialog = false">取消</VBtn>
          <VBtn color="error" :loading="deleting" @click="confirmDelete">确认删除</VBtn>
        </VCardActions>
      </VCard>
    </VDialog>

    <!-- 归档对话框 (Transfer_Dialog) -->
    <VDialog v-model="transferDialog" :max-width="smAndDown ? '100%' : 650" :fullscreen="smAndDown" scrollable>
      <VCard data-no-hover>
        <VCardTitle class="pa-4 d-flex align-center transfer-dialog-title">
          <VIcon icon="ri-archive-line" size="20" class="me-2" color="warning" />
          <span class="text-body-1 font-weight-semibold text-truncate">归档 - {{ transferItem?.Name }}</span>
          <VSpacer />
          <VBtn icon variant="text" size="small" @click="transferDialog = false">
            <VIcon icon="ri-close-line" />
          </VBtn>
        </VCardTitle>

        <VDivider />

        <VCardText class="pa-4" :class="{ 'px-3': smAndDown }">
          <!-- 源文件路径 -->
          <div v-if="transferItem?.Path" class="mb-4">
            <div class="text-caption text-medium-emphasis mb-1">源文件路径</div>
            <div class="transfer-path-box pa-2 rounded">
              <VIcon icon="ri-folder-line" size="14" class="me-1 text-medium-emphasis" />
              <span class="text-caption">{{ getTransferPath(transferItem) }}</span>
            </div>
          </div>

          <!-- 媒体类型 + 季号 -->
          <div class="d-flex align-center ga-3 mb-4 flex-wrap">
            <VChip
              :color="transferMediaType === 'movie' ? 'primary' : 'default'"
              :variant="transferMediaType === 'movie' ? 'flat' : 'outlined'"
              label
              size="large"
              @click="transferMediaType = 'movie'"
            >
              <VIcon icon="ri-film-fill" size="18" class="me-1" />
              电影
            </VChip>
            <VChip
              :color="transferMediaType === 'tv' ? 'primary' : 'default'"
              :variant="transferMediaType === 'tv' ? 'flat' : 'outlined'"
              label
              size="large"
              @click="transferMediaType = 'tv'"
            >
              <VIcon icon="ri-tv-2-fill" size="18" class="me-1" />
              剧集
            </VChip>
            <VTextField
              v-if="transferMediaType === 'tv'"
              v-model.number="transferSeason"
              type="number"
              density="compact"
              variant="outlined"
              hide-details
              placeholder="季号（可选）"
              style="max-width: 130px; flex-shrink: 0;"
            />
          </div>

          <!-- TMDB 搜索栏 -->
          <div class="d-flex align-center ga-2 mb-4">
            <VTextField
              v-model="tmdbSearchInput"
              placeholder="输入名称搜索 TMDB..."
              density="compact"
              variant="outlined"
              hide-details
              prepend-inner-icon="ri-search-line"
              clearable
              class="flex-grow-1"
              @keyup.enter="doTmdbSearch"
            />
            <VBtn color="primary" :loading="tmdbSearching" density="compact" height="40" @click="doTmdbSearch" style="min-width: 72px;">
              搜索
            </VBtn>
          </div>

          <!-- TMDB 搜索结果列表 -->
          <VProgressLinear v-if="tmdbSearching" indeterminate class="mb-3" />

          <div v-if="tmdbSearchResults.length > 0" class="tmdb-results mb-4" :style="{ maxHeight: smAndDown ? '40vh' : '280px' }">
            <VList density="compact" class="pa-0">
              <VListItem
                v-for="result in tmdbSearchResults"
                :key="result.id"
                :active="selectedTmdbResult?.id === result.id"
                color="primary"
                rounded="lg"
                class="mb-1"
                @click="selectTmdbResult(result)"
              >
                <template #prepend>
                  <VAvatar size="48" rounded="lg" class="me-2">
                    <VImg v-if="result.poster_path" :src="getTmdbPosterUrl(result.poster_path)" cover />
                    <VIcon v-else :icon="transferMediaType === 'movie' ? 'ri-film-fill' : 'ri-tv-2-fill'" size="24" />
                  </VAvatar>
                </template>
                <VListItemTitle class="text-body-2 font-weight-medium">
                  {{ result.title }}
                </VListItemTitle>
                <VListItemSubtitle class="text-caption">
                  {{ result.release_date ? result.release_date.substring(0, 4) : '未知年份' }}
                  <span v-if="result.original_title && result.original_title !== result.title" class="ms-2 text-medium-emphasis">
                    {{ result.original_title }}
                  </span>
                </VListItemSubtitle>
              </VListItem>
            </VList>
          </div>

          <div v-else-if="!tmdbSearching && tmdbSearchResults.length === 0 && tmdbSearchInput.trim()" class="text-center text-body-2 text-medium-emphasis pa-4">
            未找到匹配结果，请尝试其他关键词
          </div>

          <!-- 选中结果信息 -->
          <VAlert v-if="selectedTmdbResult" color="info" variant="tonal" density="compact" :icon="false">
            <div class="d-flex align-center flex-wrap ga-2">
              <VIcon icon="$info" size="22" color="info" class="flex-shrink-0" />
              <VAvatar v-if="selectedTmdbResult.poster_path" size="36" rounded="lg">
                <VImg :src="getTmdbPosterUrl(selectedTmdbResult.poster_path)" cover />
              </VAvatar>
              <div>
                <div class="text-body-2 font-weight-medium">
                  {{ selectedTmdbResult.title }}
                  <span v-if="selectedTmdbResult.release_date" class="text-medium-emphasis"> ({{ selectedTmdbResult.release_date.substring(0, 4) }})</span>
                </div>
                <div class="text-caption text-medium-emphasis">TMDB ID: {{ selectedTmdbResult.id }}</div>
              </div>
            </div>
          </VAlert>
        </VCardText>

        <VDivider />

        <VCardActions class="pa-4" :class="{ 'flex-column ga-2': smAndDown }">
          <VSpacer v-if="!smAndDown" />
          <VBtn variant="text" :block="smAndDown" @click="transferDialog = false">取消</VBtn>
          <VBtn
            color="warning"
            :loading="transferring"
            :disabled="!selectedTmdbResult"
            :block="smAndDown"
            @click="confirmTransfer"
          >
            <VIcon icon="ri-archive-line" size="16" class="me-1" />
            确认归档
          </VBtn>
        </VCardActions>
      </VCard>
    </VDialog>
  </div>
</template>

<style lang="scss" scoped>
// 页面特有样式（通用样式已提取到 page-common.scss）

.result-card {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  overflow: hidden;
}

.result-poster {
  position: relative;
  width: 130px;
  min-height: 195px;
  flex-shrink: 0;
  cursor: pointer;
  overflow: hidden;
}

.result-poster-empty {
  width: 130px;
  height: 195px;
  background: rgba(var(--v-theme-surface-variant), 0.3);
}

.result-poster-overlay {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transition: opacity 0.2s;
}

.result-poster:hover .result-poster-overlay {
  opacity: 1;
}

.path-chip {
  max-width: 100%;
  
  :deep(.v-chip__content) {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

// 移动端海报样式
.result-poster-mobile {
  flex-shrink: 0;
  cursor: pointer;
  width: 80px;
  height: 120px;
  overflow: hidden;
  border-radius: 6px;
}

.result-poster-empty-mobile {
  width: 80px;
  height: 120px;
  background: rgba(var(--v-theme-surface-variant), 0.3);
}

// 移动端路径显示
.mobile-path-box {
  background: rgba(var(--v-theme-on-surface), 0.05);
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  word-break: break-all;
  line-height: 1.4;
  display: flex;
  align-items: center;
}

.mobile-path-text {
  overflow: hidden;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  line-clamp: 2;
}

.mobile-action-grid {
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.mobile-action-row {
  display: grid;
  gap: 6px;
}

.mobile-action-row-links {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.mobile-action-row-actions {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.mobile-action-spacer {
  width: 100%;
}

@media (max-width: 599.98px) {
  .result-poster-overlay {
    opacity: 0;
  }
}

.tmdb-results {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 8px;
  overflow-y: auto;
}

.transfer-path-box {
  background: rgba(var(--v-theme-on-surface), 0.05);
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  word-break: break-all;
  display: flex;
  align-items: flex-start;
}

.transfer-dialog-title {
  min-height: 56px;
}
</style>

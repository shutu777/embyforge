<script setup>
import { ref, onMounted } from 'vue'
import { useDisplay } from 'vuetify'
import { useClipboard } from '@vueuse/core'
import { useRouter } from 'vue-router'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'
import { useEmbyUrl } from '@/composables/useEmbyUrl'

const snackbar = useSnackbar()
const router = useRouter()
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

// 路径替换配置
const pathReplaceConfig = ref({ clouddrive_path: '', emby_strm_path: '' })
const { copy: clipboardCopy } = useClipboard({ legacy: true })

// Symedia URL（页面加载时从配置获取）
const symediaUrl = ref('')

function goToHdhive(item) {
  if (!item?.TmdbId) return
  const type = item.Type === 'Movie' ? 'movie' : 'tv'
  const name = item.Name || item.OriginalTitle || ''
  router.push({
    path: '/hdhive-search',
    query: { tmdb_id: item.TmdbId, media_type: type, title: name },
  })
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

// 路径替换：将 Emby Strm 路径前缀替换为 CloudDrive 路径前缀
function getReplacedPath(itemPath) {
  if (!itemPath) return ''
  const embyStrm = pathReplaceConfig.value.emby_strm_path?.replace(/\/+$/, '')
  const cloudDrive = pathReplaceConfig.value.clouddrive_path?.replace(/\/+$/, '')
  if (embyStrm && cloudDrive && (itemPath === embyStrm || itemPath.startsWith(embyStrm + '/'))) {
    return cloudDrive + itemPath.slice(embyStrm.length)
  }
  return itemPath
}

// 判断路径替换是否生效
function isPathReplaceActive() {
  return !!(pathReplaceConfig.value.emby_strm_path?.trim() && pathReplaceConfig.value.clouddrive_path?.trim())
}

// 获取显示路径（如果路径替换生效则显示替换后的路径，否则显示原始路径）
function getDisplayPath(item) {
  if (!item?.Path) return ''
  return isPathReplaceActive() ? getReplacedPath(item.Path) : item.Path
}

// 复制路径到剪贴板
async function copyPath(item) {
  const path = getDisplayPath(item)
  if (!path) return
  try {
    await clipboardCopy(path)
    snackbar.success('路径已复制到剪贴板')
  } catch {
    snackbar.error('复制失败')
  }
}

// 提取文件夹路径（去掉文件名部分）
function getFolderPath(itemPath) {
  if (!itemPath) return ''
  const lastSlash = itemPath.lastIndexOf('/')
  const lastBackslash = itemPath.lastIndexOf('\\')
  const lastSep = Math.max(lastSlash, lastBackslash)
  if (lastSep > 0) {
    const lastPart = itemPath.substring(lastSep + 1)
    if (lastPart.includes('.')) {
      return itemPath.substring(0, lastSep)
    }
  }
  return itemPath
}

// 点击 Symedia 按钮：复制路径到剪贴板并打开 Symedia 文件管理器
async function onSymediaClick(item) {
  if (!item?.Path) {
    snackbar.error('该条目没有路径信息')
    return
  }
  if (!symediaUrl.value) {
    snackbar.error('请先配置 Symedia 服务地址')
    return
  }
  const folderPath = getFolderPath(item.Path)
  const replacedPath = getReplacedPath(folderPath)

  // 复制路径到剪贴板
  try {
    await clipboardCopy(replacedPath)
    snackbar.success('路径已复制，请在 Symedia 文件管理器搜索栏中粘贴')
  } catch {
    snackbar.error('复制路径失败')
  }

  // 打开 Symedia 文件管理器
  window.open(symediaUrl.value + '/#/file_manager', '_blank')
}

// 加载路径替换配置
async function fetchPathReplaceConfig() {
  try {
    const { data } = await api.get('/symedia/config')
    if (data.data?.path_replace) {
      pathReplaceConfig.value = data.data.path_replace
    }
    if (data.data?.symedia_url) {
      symediaUrl.value = data.data.symedia_url.replace(/\/+$/, '')
    }
  } catch {
    // 忽略，首次可能未配置
  }
}

onMounted(() => {
  detectEmbyUrl()
  fetchPathReplaceConfig()
})
</script>

<template>
  <div>
    <!-- 页面标题和说明 -->
    <div class="mb-6">
      <h1 class="text-h4 font-weight-bold mb-2">Emby 搜索</h1>
      <p class="text-body-1 text-medium-emphasis">
        搜索 Emby 媒体库中的电影或剧集，快速定位并管理指定条目
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
            <div class="text-body-1 font-weight-semibold">Emby 搜索</div>
            <div class="text-body-2 text-medium-emphasis">搜索 Emby 媒体库，管理电影和剧集</div>
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
                    <VChip size="x-small" variant="tonal" color="secondary" label class="path-chip path-chip-clickable" @click.stop="copyPath(item)">
                      <VIcon icon="ri-folder-line" size="12" class="me-1" />
                      {{ getDisplayPath(item) }}
                      <VIcon icon="ri-file-copy-line" size="12" class="ms-1" />
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
                    variant="text"
                    color="secondary"
                    size="small"
                    @click.stop="goToHdhive(item)"
                  >
                    <VIcon icon="ri-search-line" size="16" class="me-1" />
                    HDHive
                  </VBtn>
                  <VBtn
                    variant="text"
                    color="warning"
                    size="small"
                    @click.stop="onSymediaClick(item)"
                  >
                    <VIcon icon="ri-folder-open-line" size="16" class="me-1" />
                    Symedia
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

              <!-- 路径（独占一行，完整显示，点击复制） -->
              <div v-if="item.Path" class="px-3 pb-2">
                <div class="mobile-path-box mobile-path-box-clickable pa-2 rounded text-caption" @click.stop="copyPath(item)">
                  <VIcon icon="ri-folder-line" size="12" class="me-1 text-medium-emphasis flex-shrink-0" />
                  <span class="mobile-path-text">{{ getDisplayPath(item) }}</span>
                  <VIcon icon="ri-file-copy-line" size="12" class="ms-1 text-medium-emphasis flex-shrink-0" />
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
                    variant="text"
                    color="secondary"
                    size="small"
                    density="compact"
                    block
                    @click.stop="goToHdhive(item)"
                  >
                    <VIcon icon="ri-search-line" size="14" class="me-1" />
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
                    @click.stop="onSymediaClick(item)"
                  >
                    <VIcon icon="ri-folder-open-line" size="14" class="me-1" />
                    Symedia
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

.path-chip-clickable {
  cursor: pointer;
  transition: opacity 0.2s;

  &:hover {
    opacity: 0.8;
  }
}

.mobile-path-box-clickable {
  cursor: pointer;
  transition: background 0.2s;

  &:hover {
    background: rgba(var(--v-theme-on-surface), 0.1) !important;
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

</style>

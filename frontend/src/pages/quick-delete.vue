<script setup>
import { ref } from 'vue'
import { useDisplay } from 'vuetify'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'

const snackbar = useSnackbar()
const { smAndDown } = useDisplay()

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

async function doSearch() {
  const keyword = searchInput.value.trim()
  if (!keyword) return
  searching.value = true
  searched.value = true
  try {
    const { data } = await api.get('/quick-delete/search', { params: { keyword } })
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
    const { data } = await api.get('/quick-delete/seasons/' + seriesId)
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
    }
    const { data } = await api.post('/quick-delete/delete', body)
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
</script>

<template>
  <div>
    <!-- 搜索卡片 -->
    <VCard variant="flat" class="content-card mb-4" data-no-hover>
      <VCardText class="pa-5">
        <div class="d-flex align-center mb-4">
          <VAvatar color="error" variant="tonal" size="42" rounded="lg" class="me-3">
            <VIcon icon="ri-delete-bin-line" size="22" />
          </VAvatar>
          <div>
            <div class="text-body-1 font-weight-semibold">快速删除</div>
            <div class="text-body-2 text-medium-emphasis">搜索 Emby 媒体库，快速删除电影或剧集</div>
          </div>
        </div>

        <VRow dense>
          <VCol cols="12" sm="8">
            <VTextField
              v-model="searchInput"
              placeholder="输入电影或剧集名称搜索..."
              density="compact"
              variant="outlined"
              hide-details
              prepend-inner-icon="ri-search-line"
              clearable
              @keyup.enter="doSearch"
              @click:clear="searchResults = []; searched = false"
            />
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

      <div v-if="searchResults.length > 0" class="poster-grid">
        <div v-for="item in searchResults" :key="item.Id" class="poster-item" @click="onDeleteClick(item)">
          <!-- 海报图片 -->
          <div class="poster-wrapper">
            <img
              v-if="item.ImageUrl"
              :src="item.ImageUrl"
              :alt="item.Name"
              class="poster-img"
              loading="lazy"
            />
            <div v-else class="poster-placeholder">
              <VIcon :icon="item.Type === 'Movie' ? 'ri-film-fill' : 'ri-tv-2-fill'" size="40" color="rgba(255,255,255,0.3)" />
            </div>
            <!-- 集数角标 -->
            <div v-if="item.Type === 'Series' && getChildCount(item)" class="episode-badge">
              {{ getChildCount(item) }}
            </div>
            <!-- 悬浮删除遮罩 -->
            <div class="poster-overlay">
              <VIcon icon="ri-delete-bin-line" size="28" color="white" />
            </div>
          </div>
          <!-- 信息区域 -->
          <div class="poster-info">
            <div class="poster-name">{{ item.Name }}</div>
            <div class="poster-meta">
              {{ formatType(item.Type) }}
              <span v-if="item.ProductionYear"> · {{ item.ProductionYear }}</span>
            </div>
          </div>
        </div>
      </div>

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
.content-card {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  transform: none !important;
  box-shadow: none !important;
}

.poster-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 16px;
}

.poster-item {
  cursor: pointer;
  text-align: center;
}

.poster-wrapper {
  position: relative;
  width: 100%;
  aspect-ratio: 2 / 3;
  border-radius: 8px;
  overflow: hidden;
  background: rgba(var(--v-theme-surface-variant), 0.5);
}

.poster-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.poster-placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(var(--v-theme-surface-variant), 0.3);
}

.episode-badge {
  position: absolute;
  top: 6px;
  right: 6px;
  background: rgba(var(--v-theme-primary), 0.85);
  color: white;
  font-size: 0.75rem;
  font-weight: 600;
  padding: 2px 6px;
  border-radius: 4px;
  line-height: 1.3;
}

.poster-overlay {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.55);
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transition: opacity 0.2s;
}

.poster-item:hover .poster-overlay {
  opacity: 1;
}

.poster-info {
  padding: 8px 2px 0;
}

.poster-name {
  font-size: 0.875rem;
  font-weight: 500;
  line-height: 1.3;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.poster-meta {
  font-size: 0.75rem;
  color: rgba(var(--v-theme-on-surface), 0.6);
  margin-top: 2px;
}

@media (max-width: 599.98px) {
  .poster-grid {
    grid-template-columns: repeat(3, 1fr);
    gap: 10px;
  }

  .poster-name {
    font-size: 0.8rem;
  }

  // 移动端不需要 hover 效果，改为始终显示半透明删除图标
  .poster-overlay {
    opacity: 0;
  }
}
</style>

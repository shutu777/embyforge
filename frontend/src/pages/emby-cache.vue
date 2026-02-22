<script setup>
import { ref, computed, onMounted } from 'vue'
import { useDisplay } from 'vuetify'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'

const snackbar = useSnackbar()
const { smAndDown } = useDisplay()
const page = ref(1)
const pageSize = ref(20)
const searchInput = ref('')
const searchText = ref('')
const typeFilter = ref('')
const items = ref([])
const total = ref(0)
const loading = ref(false)
const cacheStatus = ref(null)
const editDialog = ref(false)
const editItem = ref(null)
const editForm = ref({ name: '' })
const deleteDialog = ref(false)
const deleteTarget = ref(null)
const refreshingId = ref(null)

const totalMovies = computed(() => cacheStatus.value?.total_movies || 0)
const totalSeries = computed(() => cacheStatus.value?.total_series || 0)

async function fetchCacheStatus() {
  try {
    const { data } = await api.get('/emby-cache/status')
    cacheStatus.value = data.data
  } catch (e) {
    console.error(e)
  }
}

async function fetchList() {
  loading.value = true
  try {
    const params = { page: page.value, pageSize: pageSize.value }
    if (searchText.value) params.search = searchText.value
    if (typeFilter.value) params.type = typeFilter.value
    const { data } = await api.get('/emby-cache', { params })
    items.value = data.data || []
    total.value = data.total || 0
  } catch (e) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

function doSearch() {
  searchText.value = searchInput.value.trim()
  page.value = 1
  fetchList()
}
function clearSearch() {
  searchInput.value = ''
  searchText.value = ''
  page.value = 1
  fetchList()
}
function onTypeChange() {
  page.value = 1
  fetchList()
}
function onPageChange(v) { page.value = v; fetchList() }

function openEdit(item) {
  editItem.value = item
  editForm.value = { name: item.name }
  editDialog.value = true
}
async function saveEdit() {
  if (!editItem.value) return
  try {
    await api.put('/emby-cache/' + editItem.value.id, editForm.value)
    snackbar.success('更新成功')
    editDialog.value = false
    await fetchList()
  } catch (e) { snackbar.error('更新失败') }
}

function confirmDelete(item) {
  deleteTarget.value = item
  deleteDialog.value = true
}
async function doDelete() {
  if (!deleteTarget.value) return
  try {
    await api.delete('/emby-cache/' + deleteTarget.value.id)
    snackbar.success('删除成功')
    deleteDialog.value = false
    await Promise.all([fetchList(), fetchCacheStatus()])
  } catch (e) { snackbar.error('删除失败') }
}

async function refreshItem(item) {
  refreshingId.value = item.id
  try {
    const { data } = await api.post('/emby-cache/' + item.id + '/refresh')
    if (data.deleted) {
      snackbar.success('Emby 中已不存在该条目，已删除本地缓存')
    } else {
      snackbar.success('刷新成功')
    }
    await Promise.all([fetchList(), fetchCacheStatus()])
  } catch (e) { snackbar.error('刷新失败') }
  finally { refreshingId.value = null }
}

function formatType(type) {
  return type === 'Movie' ? '电影' : type === 'Series' ? '剧集' : type
}
function formatSize(bytes) {
  if (!bytes) return '-'
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB'
  if (bytes < 1073741824) return (bytes / 1048576).toFixed(1) + ' MB'
  return (bytes / 1073741824).toFixed(2) + ' GB'
}
function formatTime(t) {
  if (!t) return '-'
  return new Date(t).toLocaleString('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  })
}
function getProviderIds(item) {
  try {
    const ids = JSON.parse(item.provider_ids || '{}')
    return Object.entries(ids).map(([k, v]) => `${k}: ${v}`).join(', ') || '-'
  } catch { return '-' }
}

onMounted(async () => { await Promise.all([fetchList(), fetchCacheStatus()]) })
</script>

<template>
  <div>
    <!-- 统计卡片 -->
    <VRow class="mb-4">
      <VCol cols="12" sm="4">
        <VCard class="stat-card" style="height: 120px;">
          <VCardText class="d-flex align-center justify-space-between h-100 pa-5">
            <div>
              <div class="text-body-2 text-medium-emphasis mb-1">电影</div>
              <div class="text-h4 font-weight-bold">{{ totalMovies.toLocaleString() }}</div>
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
              <div class="text-body-2 text-medium-emphasis mb-1">剧集</div>
              <div class="text-h4 font-weight-bold">{{ totalSeries.toLocaleString() }}</div>
            </div>
            <div class="stat-icon" style="background: #8b5cf618;">
              <VIcon icon="ri-tv-2-fill" color="#8b5cf6" size="24" />
            </div>
          </VCardText>
        </VCard>
      </VCol>
      <VCol cols="12" sm="4">
        <VCard class="stat-card" style="height: 120px;">
          <VCardText class="d-flex align-center justify-space-between h-100 pa-5">
            <div>
              <div class="text-body-2 text-medium-emphasis mb-1">总计</div>
              <div class="text-h4 font-weight-bold">{{ (totalMovies + totalSeries).toLocaleString() }}</div>
            </div>
            <div class="stat-icon" style="background: #10b98118;">
              <VIcon icon="ri-database-2-fill" color="#10b981" size="24" />
            </div>
          </VCardText>
        </VCard>
      </VCol>
    </VRow>

    <!-- 缓存列表 -->
    <VCard variant="flat" class="content-card" data-no-hover>
      <VCardText class="pa-5">
        <div class="d-flex align-center mb-4">
          <VAvatar color="primary" variant="tonal" size="42" rounded="lg" class="me-3">
            <VIcon icon="ri-hard-drive-2-line" size="22" />
          </VAvatar>
          <div>
            <div class="text-body-1 font-weight-semibold">Emby 缓存列表</div>
            <div class="text-body-2 text-medium-emphasis">本地缓存的电影和剧集数据</div>
          </div>
        </div>

        <!-- 搜索和筛选 -->
        <VRow class="mb-4" dense>
          <VCol cols="12" sm="6">
            <VTextField
              v-model="searchInput"
              placeholder="搜索名称..."
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
              v-model="typeFilter"
              :items="[
                { title: '全部类型', value: '' },
                { title: '电影', value: 'Movie' },
                { title: '剧集', value: 'Series' },
              ]"
              density="compact"
              variant="outlined"
              hide-details
              @update:model-value="onTypeChange"
            />
          </VCol>
        </VRow>

        <!-- 移动端：卡片布局 -->
        <div v-if="smAndDown && items.length > 0" class="mobile-items">
          <div v-for="item in items" :key="item.id" class="mobile-item pa-3">
            <div class="d-flex align-center justify-space-between mb-1">
              <div class="d-flex align-center gap-2" style="min-width: 0; flex: 1;">
                <VChip size="x-small" :color="item.type === 'Movie' ? 'success' : 'info'">
                  {{ formatType(item.type) }}
                </VChip>
                <span class="text-body-2 font-weight-medium text-truncate">{{ item.name }}</span>
              </div>
            </div>
            <div class="text-caption text-medium-emphasis mb-1">
              <span v-if="item.type === 'Series' && item.child_count">{{ item.child_count }} 季</span>
              <span v-if="item.file_size"> · {{ formatSize(item.file_size) }}</span>
              <span> · {{ formatTime(item.cached_at) }}</span>
            </div>
            <div class="text-caption text-medium-emphasis mb-2" style="word-break: break-all;">
              {{ getProviderIds(item) }}
            </div>
            <div class="d-flex justify-end gap-2">
              <VBtn size="x-small" variant="tonal" color="primary" @click="openEdit(item)">
                <VIcon icon="ri-edit-line" size="12" class="me-1" />
                编辑
              </VBtn>
              <VBtn size="x-small" variant="tonal" color="warning" :loading="refreshingId === item.id" @click="refreshItem(item)">
                <VIcon icon="ri-refresh-line" size="12" class="me-1" />
                刷新
              </VBtn>
              <VBtn size="x-small" variant="tonal" color="error" @click="confirmDelete(item)">
                <VIcon icon="ri-delete-bin-line" size="12" class="me-1" />
                删除
              </VBtn>
            </div>
          </div>
        </div>

        <!-- 桌面端：表格布局 -->
        <div v-else-if="items.length > 0" class="table-responsive">
          <VTable density="compact">
            <thead>
              <tr>
                <th>名称</th>
                <th style="width: 70px;">类型</th>
                <th>Provider IDs</th>
                <th style="width: 90px;">大小</th>
                <th style="width: 160px;">缓存时间</th>
                <th style="width: 130px; text-align: center;">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in items" :key="item.id">
                <td>
                  <span class="text-body-2">{{ item.name }}</span>
                  <span v-if="item.type === 'Series' && item.child_count" class="text-caption text-medium-emphasis ms-1">({{ item.child_count }} 季)</span>
                </td>
                <td>
                  <VChip size="x-small" :color="item.type === 'Movie' ? 'success' : 'info'">
                    {{ formatType(item.type) }}
                  </VChip>
                </td>
                <td class="text-caption provider-cell">
                  {{ getProviderIds(item) }}
                </td>
                <td class="text-caption">{{ formatSize(item.file_size) }}</td>
                <td class="text-caption">{{ formatTime(item.cached_at) }}</td>
                <td>
                  <div class="d-flex align-center justify-center action-btns">
                    <VTooltip text="编辑" location="top">
                      <template #activator="{ props }">
                        <VBtn v-bind="props" size="small" icon variant="text" color="primary" @click="openEdit(item)">
                          <VIcon icon="ri-edit-line" size="16" />
                        </VBtn>
                      </template>
                    </VTooltip>
                    <VTooltip text="从 Emby 刷新" location="top">
                      <template #activator="{ props }">
                        <VBtn v-bind="props" size="small" icon variant="text" color="warning" :loading="refreshingId === item.id" @click="refreshItem(item)">
                          <VIcon icon="ri-refresh-line" size="16" />
                        </VBtn>
                      </template>
                    </VTooltip>
                    <VTooltip text="删除" location="top">
                      <template #activator="{ props }">
                        <VBtn v-bind="props" size="small" icon variant="text" color="error" @click="confirmDelete(item)">
                          <VIcon icon="ri-delete-bin-line" size="16" />
                        </VBtn>
                      </template>
                    </VTooltip>
                  </div>
                </td>
              </tr>
            </tbody>
          </VTable>
        </div>

        <div v-else-if="!loading" class="text-center pa-4 text-body-2 text-medium-emphasis">
          {{ searchText || typeFilter ? '没有匹配的结果' : '暂无缓存数据，请先同步 Emby 媒体库' }}
        </div>
        <VProgressLinear v-if="loading" indeterminate class="mt-2" />
        <div v-if="total > pageSize" class="d-flex justify-center mt-4">
          <VPagination v-model="page" :length="Math.ceil(total / pageSize)" @update:model-value="onPageChange" />
        </div>
      </VCardText>
    </VCard>

    <!-- 编辑对话框 -->
    <VDialog v-model="editDialog" max-width="450">
      <VCard data-no-hover>
        <VCardTitle class="text-body-1 font-weight-semibold pa-4">编辑缓存条目</VCardTitle>
        <VCardText class="pa-4 pt-0">
          <VTextField v-model="editForm.name" label="名称" density="compact" variant="outlined" />
        </VCardText>
        <VCardActions class="pa-4 pt-0">
          <VSpacer />
          <VBtn variant="text" @click="editDialog = false">取消</VBtn>
          <VBtn color="primary" @click="saveEdit">保存</VBtn>
        </VCardActions>
      </VCard>
    </VDialog>

    <!-- 删除确认对话框 -->
    <VDialog v-model="deleteDialog" max-width="400">
      <VCard data-no-hover>
        <VCardTitle class="text-body-1 font-weight-semibold pa-4">确认删除</VCardTitle>
        <VCardText class="pa-4 pt-0">
          确定要删除 {{ deleteTarget?.name }} 的缓存数据吗？
          <span v-if="deleteTarget?.type === 'Series'" class="text-error">（将同时删除关联的剧集和季数据）</span>
        </VCardText>
        <VCardActions class="pa-4 pt-0">
          <VSpacer />
          <VBtn variant="text" @click="deleteDialog = false">取消</VBtn>
          <VBtn color="error" @click="doDelete">删除</VBtn>
        </VCardActions>
      </VCard>
    </VDialog>
  </div>
</template>

<style lang="scss" scoped>
.stat-card {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
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

.content-card {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  transform: none !important;
  box-shadow: none !important;
}

.mobile-items {
  .mobile-item {
    border-block-end: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));

    &:last-child {
      border-block-end: none;
    }
  }
}

.table-responsive {
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
}

.provider-cell {
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.action-btns {
  gap: 2px;
}

@media (max-width: 599.98px) {
  .stat-icon {
    width: 40px;
    height: 40px;
  }
}
</style>

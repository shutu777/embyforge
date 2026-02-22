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
const groups = ref([])
const total = ref(0)
const loading = ref(false)
const cacheStatus = ref(null)
const editDialog = ref(false)
const editItem = ref(null)
const editForm = ref({ episode_count: 0, season_name: '' })
const clearDialog = ref(false)
const clearing = ref(false)
const deleteShowDialog = ref(false)
const deleteShowTarget = ref(null)

const totalShows = computed(() => cacheStatus.value?.total_shows || 0)
const totalRecords = computed(() => cacheStatus.value?.total_records || 0)

async function fetchCacheStatus() {
  try {
    const { data } = await api.get('/tmdb-cache/status')
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
    const { data } = await api.get('/tmdb-cache', { params })
    groups.value = data.data || []
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
function onPageChange(v) { page.value = v; fetchList() }
function openEdit(s) {
  editItem.value = s
  editForm.value = { episode_count: s.episode_count, season_name: s.season_name }
  editDialog.value = true
}
async function saveEdit() {
  if (!editItem.value) return
  try {
    await api.put('/tmdb-cache/' + editItem.value.id, editForm.value)
    snackbar.success('更新成功')
    editDialog.value = false
    await fetchList()
  } catch (e) { snackbar.error('更新失败') }
}
async function deleteSeason(id) {
  try {
    await api.delete('/tmdb-cache/' + id)
    snackbar.success('删除成功')
    await Promise.all([fetchList(), fetchCacheStatus()])
  } catch (e) { snackbar.error('删除失败') }
}
function confirmDeleteShow(g) { deleteShowTarget.value = g; deleteShowDialog.value = true }
async function deleteShow() {
  if (!deleteShowTarget.value) return
  try {
    await api.delete('/tmdb-cache/show/' + deleteShowTarget.value.tmdb_id)
    snackbar.success('删除成功')
    deleteShowDialog.value = false
    await Promise.all([fetchList(), fetchCacheStatus()])
  } catch (e) { snackbar.error('删除失败') }
}
async function clearAll() {
  clearing.value = true
  try {
    await api.post('/tmdb-cache/clear')
    snackbar.success('清空成功')
    clearDialog.value = false
    await Promise.all([fetchList(), fetchCacheStatus()])
  } catch (e) { snackbar.error('清空失败') }
  finally { clearing.value = false }
}
function openInTmdb(tmdbId) {
  window.open('https://www.themoviedb.org/tv/' + tmdbId, '_blank')
}
function formatTime(t) {
  if (!t) return '-'
  return new Date(t).toLocaleString('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  })
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
              <div class="text-body-2 text-medium-emphasis mb-1">缓存节目数</div>
              <div class="text-h4 font-weight-bold">{{ totalShows.toLocaleString() }}</div>
            </div>
            <div class="stat-icon" style="background: #6366f118;">
              <VIcon icon="ri-movie-2-fill" color="#6366f1" size="24" />
            </div>
          </VCardText>
        </VCard>
      </VCol>
      <VCol cols="12" sm="4">
        <VCard class="stat-card" style="height: 120px;">
          <VCardText class="d-flex align-center justify-space-between h-100 pa-5">
            <div>
              <div class="text-body-2 text-medium-emphasis mb-1">缓存记录数</div>
              <div class="text-h4 font-weight-bold">{{ totalRecords.toLocaleString() }}</div>
            </div>
            <div class="stat-icon" style="background: #8b5cf618;">
              <VIcon icon="ri-database-2-fill" color="#8b5cf6" size="24" />
            </div>
          </VCardText>
        </VCard>
      </VCol>
      <VCol cols="12" sm="4">
        <VCard class="stat-card d-flex align-center justify-center" style="height: 120px;">
          <VBtn color="error" variant="tonal" :disabled="totalRecords === 0" @click="clearDialog = true">
            <VIcon icon="ri-delete-bin-line" class="me-1" />
            清空全部缓存
          </VBtn>
        </VCard>
      </VCol>
    </VRow>

    <!-- 缓存列表 -->
    <VCard variant="flat" class="content-card" data-no-hover>
      <VCardText class="pa-5">
        <div class="d-flex align-center mb-4">
          <VAvatar color="primary" variant="tonal" size="42" rounded="lg" class="me-3">
            <VIcon icon="ri-database-2-line" size="22" />
          </VAvatar>
          <div>
            <div class="text-body-1 font-weight-semibold">TMDB 缓存列表</div>
            <div class="text-body-2 text-medium-emphasis">共 {{ total.toLocaleString() }} 个节目的季集数据已缓存</div>
          </div>
        </div>

        <!-- 搜索栏 -->
        <VRow class="mb-4" dense>
          <VCol cols="12" sm="6">
            <VTextField
              v-model="searchInput"
              placeholder="搜索节目名称（中/英文）或 TMDB ID..."
              density="compact"
              variant="outlined"
              hide-details
              prepend-inner-icon="ri-search-line"
              clearable
              @keyup.enter="doSearch"
              @click:clear="clearSearch"
            />
          </VCol>
        </VRow>

        <!-- 数据列表 -->
        <div v-if="groups.length > 0">
          <VExpansionPanels variant="accordion">
            <VExpansionPanel v-for="group in groups" :key="group.tmdb_id">
              <VExpansionPanelTitle>
                <div class="panel-title-content">
                  <div class="d-flex align-center gap-2">
                    <VChip size="small" color="primary">{{ group.season_count }} 季</VChip>
                    <VChip size="small" color="info" variant="tonal">TMDB {{ group.tmdb_id }}</VChip>
                  </div>
                  <span class="text-body-2 font-weight-medium panel-title-name">{{ group.name }}</span>
                </div>
              </VExpansionPanelTitle>
              <VExpansionPanelText>
                <div class="d-flex justify-end mb-2 gap-2">
                  <VBtn size="small" variant="text" color="info" @click.stop="openInTmdb(group.tmdb_id)">
                    <VIcon icon="ri-external-link-line" size="14" class="me-1" />
                    在 TMDB 中查看
                  </VBtn>
                  <VBtn size="small" variant="text" color="error" @click.stop="confirmDeleteShow(group)">
                    <VIcon icon="ri-delete-bin-line" size="14" class="me-1" />
                    删除该节目缓存
                  </VBtn>
                </div>
                <!-- 移动端：卡片布局 -->
                <div v-if="smAndDown" class="mobile-items">
                  <div v-for="season in group.seasons" :key="season.id" class="mobile-item pa-3">
                    <div class="d-flex align-center justify-space-between mb-1">
                      <span class="text-body-2 font-weight-medium">Season {{ season.season_number }}</span>
                      <span class="text-body-2">{{ season.episode_count }} 集</span>
                    </div>
                    <div class="text-caption text-medium-emphasis mb-1">{{ season.season_name || '-' }}</div>
                    <div class="d-flex align-center justify-space-between">
                      <span class="text-caption text-medium-emphasis">{{ formatTime(season.cached_at) }}</span>
                      <div class="d-flex gap-1">
                        <VBtn size="x-small" variant="text" color="primary" @click="openEdit(season)">
                          <VIcon icon="ri-edit-line" size="14" />
                        </VBtn>
                        <VBtn size="x-small" variant="text" color="error" @click="deleteSeason(season.id)">
                          <VIcon icon="ri-delete-bin-line" size="14" />
                        </VBtn>
                      </div>
                    </div>
                  </div>
                </div>
                <!-- 桌面端：表格布局 -->
                <VTable v-else density="compact">
                  <thead>
                    <tr>
                      <th>季</th>
                      <th>季名称</th>
                      <th>集数</th>
                      <th>缓存时间</th>
                      <th style="width: 140px;">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="season in group.seasons" :key="season.id">
                      <td>Season {{ season.season_number }}</td>
                      <td>{{ season.season_name || '-' }}</td>
                      <td>{{ season.episode_count }}</td>
                      <td>{{ formatTime(season.cached_at) }}</td>
                      <td>
                        <VBtn size="x-small" variant="text" color="primary" @click="openEdit(season)">
                          <VIcon icon="ri-edit-line" size="14" />
                        </VBtn>
                        <VBtn size="x-small" variant="text" color="error" @click="deleteSeason(season.id)">
                          <VIcon icon="ri-delete-bin-line" size="14" />
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
          {{ searchText ? '没有匹配的结果' : '暂无缓存数据，分析异常映射时会自动缓存 TMDB 数据' }}
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
        <VCardTitle class="text-body-1 font-weight-semibold pa-4">编辑缓存记录</VCardTitle>
        <VCardText class="pa-4 pt-0">
          <VTextField v-model="editForm.season_name" label="季名称" density="compact" variant="outlined" class="mb-3" />
          <VTextField v-model.number="editForm.episode_count" label="集数" type="number" density="compact" variant="outlined" />
        </VCardText>
        <VCardActions class="pa-4 pt-0">
          <VSpacer />
          <VBtn variant="text" @click="editDialog = false">取消</VBtn>
          <VBtn color="primary" @click="saveEdit">保存</VBtn>
        </VCardActions>
      </VCard>
    </VDialog>

    <!-- 删除节目确认对话框 -->
    <VDialog v-model="deleteShowDialog" max-width="400">
      <VCard data-no-hover>
        <VCardTitle class="text-body-1 font-weight-semibold pa-4">确认删除</VCardTitle>
        <VCardText class="pa-4 pt-0">确定要删除 {{ deleteShowTarget?.name }} (TMDB {{ deleteShowTarget?.tmdb_id }}) 的所有缓存数据吗？</VCardText>
        <VCardActions class="pa-4 pt-0">
          <VSpacer />
          <VBtn variant="text" @click="deleteShowDialog = false">取消</VBtn>
          <VBtn color="error" @click="deleteShow">删除</VBtn>
        </VCardActions>
      </VCard>
    </VDialog>

    <!-- 清空全部确认对话框 -->
    <VDialog v-model="clearDialog" max-width="400">
      <VCard data-no-hover>
        <VCardTitle class="text-body-1 font-weight-semibold pa-4">确认清空</VCardTitle>
        <VCardText class="pa-4 pt-0">确定要清空所有 TMDB 缓存数据吗？下次分析异常映射时将重新从 TMDB 拉取数据。</VCardText>
        <VCardActions class="pa-4 pt-0">
          <VSpacer />
          <VBtn variant="text" @click="clearDialog = false">取消</VBtn>
          <VBtn color="error" :loading="clearing" @click="clearAll">清空</VBtn>
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
}
</style>

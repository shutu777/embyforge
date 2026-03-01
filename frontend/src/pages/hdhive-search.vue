<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'

const snackbar = useSnackbar()
const route = useRoute()
const router = useRouter()

// 视图模式: 'search' = 外围搜索视图, 'detail' = 详情视图, 'loading' = 外部跳转加载中
const viewMode = ref('search')

// 来源标记：是否从外部页面（如Emby搜索）跳转而来
const fromExternal = ref(false)

// 搜索相关
const searchInput = ref('')
const searchResults = ref([])
const searching = ref(false)
const searched = ref(false)

// 详情相关
const detailLoading = ref(false)
const selectedItem = ref(null)
const resources = ref([])

// 按钮加载状态
const unlockingMap = ref({})
const viewingMap = ref({})

// TMDB 图片基础地址
const TMDB_IMAGE_BASE = 'https://image.tmdb.org/t/p'

// 是否处于详情视图
const isDetailView = computed(() => viewMode.value === 'detail')

// 页面加载时检查 URL 参数，支持从 Emby 搜索跳转
onMounted(async () => {
  const { tmdb_id, media_type, title } = route.query
  if (tmdb_id && media_type) {
    fromExternal.value = true
    // 先显示加载状态，避免空白页面
    viewMode.value = 'loading'

    // 通过 TMDB 详情 API 获取完整信息（海报/背景/评分/简介）
    let item = {
      id: Number(tmdb_id),
      media_type: media_type,
      name: title || '',
      title: title || '',
    }
    try {
      const { data } = await api.get('/hdhive/tmdb-info', {
        params: { tmdb_id, media_type },
      })
      if (data.data) {
        const d = data.data
        item = {
          id: Number(tmdb_id),
          media_type,
          name: d.name || d.title || title || '',
          title: d.title || d.name || title || '',
          original_name: d.original_name || d.original_title || '',
          original_title: d.original_title || d.original_name || '',
          poster_path: d.poster_path || '',
          backdrop_path: d.backdrop_path || '',
          overview: d.overview || '',
          vote_average: d.vote_average || 0,
          vote_count: d.vote_count || 0,
          release_date: d.release_date || '',
          first_air_date: d.first_air_date || '',
        }
      }
    } catch (e) {
      // 获取TMDB信息失败不影响流程，使用基础信息
    }
    onCardClick(item)
  }
})

// 搜索
async function doSearch() {
  const keyword = searchInput.value.trim()
  if (!keyword) return

  // 搜索时切回外围视图
  viewMode.value = 'search'
  selectedItem.value = null
  resources.value = []

  searching.value = true
  searched.value = true
  try {
    const { data } = await api.get('/hdhive/search', { params: { query: keyword } })
    searchResults.value = (data.data?.results || []).filter(
      item => item.media_type !== 'person' && item.poster_path
    )
  } catch (e) {
    const msg = e.response?.data?.message || e.message
    snackbar.error('搜索失败: ' + msg)
    searchResults.value = []
  } finally {
    searching.value = false
  }
}

// 返回搜索视图
function goBack() {
  if (fromExternal.value) {
    // 从外部页面（如Emby搜索）跳转来的，返回上一页
    router.back()
  } else {
    // 从本页搜索进入的详情，回到搜索列表
    viewMode.value = 'search'
    selectedItem.value = null
    resources.value = []
    // 清除URL上的查询参数
    if (route.query.tmdb_id) {
      router.replace({ path: '/hdhive-search' })
    }
  }
}

// 获取显示名称
function getDisplayName(item) {
  if (!item) return '未知'
  return item.name || item.title || item.original_name || item.original_title || '未知'
}

// 获取年份
function getYear(item) {
  if (!item) return ''
  const date = item.first_air_date || item.release_date || ''
  return date ? date.substring(0, 4) : ''
}

// 获取媒体类型显示
function getMediaTypeLabel(item) {
  if (!item) return ''
  if (item.media_type === 'tv') return '剧集'
  if (item.media_type === 'movie') return '电影'
  return item.media_type || ''
}

// 获取海报URL
function getPosterUrl(path, size = 'w342') {
  if (!path) return ''
  return `${TMDB_IMAGE_BASE}/${size}${path}`
}

// 获取背景图URL
function getBackdropUrl(path) {
  if (!path) return ''
  return `${TMDB_IMAGE_BASE}/w1280${path}`
}

// 点击搜索结果卡片 → 切换到详情视图
async function onCardClick(item) {
  selectedItem.value = item
  viewMode.value = 'detail'
  detailLoading.value = true
  resources.value = []

  try {
    const mediaType = item.media_type === 'movie' ? 'movie' : 'tv'
    const { data } = await api.get('/hdhive/detail', {
      params: { tmdb_id: item.id, media_type: mediaType },
    })
    resources.value = data.data || []
  } catch (e) {
    snackbar.error('获取资源详情失败: ' + (e.response?.data?.message || e.message))
  } finally {
    detailLoading.value = false
  }
}

// 查看资源（解锁并打开链接）
async function viewResource(resource) {
  viewingMap.value[resource.id] = true
  // 同步打开空白窗口（在用户点击上下文中，避免被浏览器拦截）
  const win = window.open('about:blank', '_blank')
  try {
    const { data } = await api.post('/hdhive/unlock', { resource_id: resource.id })
    const result = data.data
    if (result?.success && result?.url) {
      snackbar.success(result.message || '解锁成功')
      if (win) {
        win.location.href = result.url
      } else {
        window.open(result.url, '_blank')
      }
    } else {
      if (win) win.close()
      snackbar.error(result?.message || '解锁失败')
    }
  } catch (e) {
    if (win) win.close()
    snackbar.error('解锁失败: ' + (e.response?.data?.message || e.message))
  } finally {
    viewingMap.value[resource.id] = false
  }
}

// 转存资源（解锁 → 获取分享URL → 调用115 WebAPI转存）
async function saveResource(resource) {
  unlockingMap.value[resource.id] = true
  try {
    // 第一步：解锁获取分享链接
    const { data: unlockData } = await api.post('/hdhive/unlock', { resource_id: resource.id })
    const unlockResult = unlockData.data
    if (!unlockResult?.success || !unlockResult?.url) {
      snackbar.error(unlockResult?.message || '解锁失败，无法获取分享链接')
      return
    }

    // 第二步：调用115转存API
    const { data: transferData } = await api.post('/pan115/transfer', { share_url: unlockResult.url })
    const transferResult = transferData.data
    if (transferResult?.success) {
      snackbar.success(transferResult.message || '转存成功')
    } else {
      snackbar.error(transferResult?.message || '转存失败')
    }
  } catch (e) {
    const msg = e.response?.data?.message || e.message
    if (msg.includes('Cookie') || msg.includes('配置')) {
      snackbar.error('请先在 115 配置页面设置 Cookie 和存储目录')
    } else {
      snackbar.error('转存失败: ' + msg)
    }
  } finally {
    unlockingMap.value[resource.id] = false
  }
}

// 从 tags 中提取分辨率信息（4K、1080P 等），用 · 连接
function getResolutionText(res) {
  if (!res.tags) return ''
  const resolutions = res.tags.filter(t => {
    const l = t.toLowerCase()
    return l.includes('4k') || l.includes('1080') || l.includes('2160') || l.includes('720')
  })
  return resolutions.join(' · ')
}

// 从 tags 中提取字幕信息（语言+类型），用 · 连接
function getSubtitleText(res) {
  if (!res.tags) return ''
  const subs = res.tags.filter(t => {
    const l = t.toLowerCase()
    return l.includes('中') || l.includes('简') || l.includes('繁') || l.includes('英')
      || l.includes('内封') || l.includes('内嵌') || l.includes('外挂')
  })
  return subs.join(' · ')
}

// 从 tags 中提取来源信息（WEB-DL、蓝光等），用 · 连接
function getSourceText(res) {
  if (!res.tags) return ''
  const sources = res.tags.filter(t => {
    const l = t.toLowerCase()
    return l.includes('web') || l.includes('blu') || l.includes('remux')
      || l.includes('encode') || l.includes('原盘')
  })
  return sources.join(' · ')
}
</script>

<template>
  <div>
    <!-- 外部跳转加载状态 -->
    <template v-if="viewMode === 'loading'">
      <div class="d-flex flex-column align-center justify-center" style="min-height: 60vh;">
        <VProgressCircular indeterminate color="warning" size="56" width="5" class="mb-5" />
        <div class="text-h6 font-weight-medium mb-2">正在加载资源信息...</div>
        <div class="text-body-2 text-medium-emphasis">正在从 HDHive 获取数据</div>
      </div>
    </template>

    <template v-else>
    <!-- 页面标题 -->
    <div class="mb-6">
      <h1 class="text-h4 font-weight-bold mb-2">HDHive 搜索</h1>
      <p class="text-body-1 text-medium-emphasis">
        搜索 HDHive 资源，查看和解锁 115 网盘资源
      </p>
    </div>

    <!-- 搜索卡片（始终显示） -->
    <VCard variant="flat" class="content-card mb-7" data-no-hover>
      <VCardText class="pa-5">
        <div class="d-flex align-center mb-4">
          <VAvatar color="warning" variant="tonal" size="42" rounded="lg" class="me-3">
            <VIcon icon="ri-fire-line" size="22" />
          </VAvatar>
          <div>
            <div class="text-body-1 font-weight-semibold">HDHive 搜索</div>
            <div class="text-body-2 text-medium-emphasis">搜索影视资源，支持 TMDB ID</div>
          </div>
        </div>

        <VRow dense>
          <VCol cols="12" sm="8">
            <form @submit.capture.stop.prevent="doSearch">
              <VTextField
                v-model="searchInput"
                placeholder="输入影视名称搜索..."
                density="compact"
                variant="outlined"
                hide-details
                prepend-inner-icon="ri-search-line"
                clearable
                enterkeyhint="search"
                @keydown.enter.capture.stop.prevent="doSearch"
                @keyup.enter.capture.stop.prevent="doSearch"
                @click:clear="searchResults = []; searched = false; goBack()"
              />
            </form>
          </VCol>
          <VCol cols="12" sm="2">
            <VBtn color="warning" block :loading="searching" @click="doSearch">
              <VIcon icon="ri-search-line" class="me-1" />
              搜索
            </VBtn>
          </VCol>
        </VRow>
      </VCardText>
    </VCard>

    <!-- ==================== 外围搜索视图 ==================== -->
    <template v-if="!isDetailView">
      <template v-if="searched">
        <div class="text-body-2 text-medium-emphasis mb-3">
          找到 {{ searchResults.length }} 个结果
        </div>

        <VProgressLinear v-if="searching" indeterminate color="warning" class="mb-3" />

        <VRow v-if="searchResults.length > 0">
          <VCol
            v-for="item in searchResults"
            :key="item.id"
            cols="6"
            sm="4"
            md="3"
            lg="2"
          >
            <VCard
              class="hdhive-card"
              hover
              @click="onCardClick(item)"
            >
              <div class="hdhive-poster-wrap">
                <VImg
                  v-if="item.poster_path"
                  :src="getPosterUrl(item.poster_path)"
                  :alt="getDisplayName(item)"
                  cover
                  class="hdhive-poster"
                />
                <div v-else class="hdhive-poster-empty d-flex align-center justify-center">
                  <VIcon
                    :icon="item.media_type === 'movie' ? 'ri-film-line' : 'ri-tv-line'"
                    size="48"
                    color="rgba(255,255,255,0.2)"
                  />
                </div>
                <div v-if="item.vote_average" class="hdhive-rating">
                  <VIcon icon="ri-star-fill" size="12" color="amber" class="me-1" />
                  <span>{{ item.vote_average.toFixed(1) }}</span>
                </div>
                <div class="hdhive-type-badge">
                  <VChip
                    size="x-small"
                    :color="item.media_type === 'movie' ? 'primary' : 'info'"
                    variant="flat"
                    label
                  >
                    {{ getMediaTypeLabel(item) }}
                  </VChip>
                </div>
              </div>

              <VCardText class="pa-3">
                <div class="text-body-1 font-weight-bold text-truncate text-high-emphasis">
                  {{ getDisplayName(item) }}
                </div>
                <div class="d-flex align-center ga-2 mt-1">
                  <span v-if="getYear(item)" class="text-body-2 text-medium-emphasis">
                    {{ getYear(item) }}
                  </span>
                  <VChip size="x-small" variant="tonal" color="success" label>
                    TMDB {{ item.id }}
                  </VChip>
                </div>
                <div
                  v-if="item.overview"
                  class="text-caption text-medium-emphasis mt-2 hdhive-overview"
                >
                  {{ item.overview }}
                </div>
              </VCardText>
            </VCard>
          </VCol>
        </VRow>

        <div v-else-if="!searching" class="text-center pa-8 text-body-2 text-medium-emphasis">
          没有找到匹配的结果
        </div>
      </template>
    </template>

    <!-- ==================== 详情视图 ==================== -->
    <template v-if="isDetailView && selectedItem">
      <!-- 返回按钮 -->
      <VBtn
        variant="text"
        color="warning"
        class="mb-4"
        prepend-icon="ri-arrow-left-line"
        @click="goBack"
      >
        返回搜索结果
      </VBtn>

      <!-- 影视详情头部（背景图+海报+信息） -->
      <VCard class="detail-hero mb-6" variant="flat" data-no-hover>
        <!-- 背景图 -->
        <div class="detail-backdrop">
          <VImg
            v-if="selectedItem.backdrop_path"
            :src="getBackdropUrl(selectedItem.backdrop_path)"
            cover
          />
          <div v-else class="detail-backdrop-empty" />
          <div class="detail-backdrop-overlay" />
        </div>

        <!-- 内容层 -->
        <div class="detail-content pa-5">
          <div class="d-flex flex-column flex-sm-row ga-5">
            <!-- 海报 -->
            <div class="detail-poster flex-shrink-0">
              <div
                v-if="selectedItem.poster_path"
                class="rounded-lg elevation-8"
                style="width: 200px; aspect-ratio: 2/3; overflow: hidden;"
              >
                <VImg
                  :src="getPosterUrl(selectedItem.poster_path, 'w500')"
                  :alt="getDisplayName(selectedItem)"
                  cover
                  height="100%"
                />
              </div>
              <div
                v-else
                class="d-flex align-center justify-center rounded-lg"
                style="width: 200px; aspect-ratio: 2/3; background: rgba(255,255,255,0.05);"
              >
                <VIcon
                  :icon="selectedItem.media_type === 'movie' ? 'ri-film-line' : 'ri-tv-line'"
                  size="64"
                  color="rgba(255,255,255,0.2)"
                />
              </div>
            </div>

            <!-- 信息区 -->
            <div class="flex-grow-1" style="min-width: 0;">
              <h2 class="text-h4 font-weight-bold mb-1">
                {{ getDisplayName(selectedItem) }}
              </h2>
              <div class="text-body-1 text-medium-emphasis mb-3">
                {{ getYear(selectedItem) ? `(${getYear(selectedItem)})` : '' }}
              </div>

              <!-- 标签行 -->
              <div class="d-flex flex-wrap ga-2 mb-4">
                <VChip
                  :color="selectedItem.media_type === 'movie' ? 'primary' : 'info'"
                  variant="flat"
                  size="small"
                  label
                >
                  {{ getMediaTypeLabel(selectedItem) }}
                </VChip>
                <VChip v-if="selectedItem.vote_average" variant="tonal" size="small" label>
                  <VIcon icon="ri-star-fill" size="14" color="amber" class="me-1" />
                  {{ selectedItem.vote_average.toFixed(1) }}/10
                  <span v-if="selectedItem.vote_count" class="ms-1 text-medium-emphasis">
                    ({{ selectedItem.vote_count }})
                  </span>
                </VChip>
                <VChip variant="tonal" color="success" size="small" label>
                  TMDB {{ selectedItem.id }}
                </VChip>
              </div>

              <!-- 剧情简介 -->
              <div v-if="selectedItem.overview" class="mb-2">
                <div class="text-body-2 font-weight-semibold mb-1">剧情简介</div>
                <div class="text-body-2 text-medium-emphasis" style="line-height: 1.7;">
                  {{ selectedItem.overview }}
                </div>
              </div>
            </div>
          </div>
        </div>
      </VCard>

      <!-- 115 资源区域 -->
      <div class="mb-4">
        <div class="d-flex align-center mb-4">
          <VIcon icon="ri-cloud-line" size="22" color="warning" class="me-2" />
          <span class="text-h6 font-weight-semibold">115 网盘资源</span>
          <VChip v-if="!detailLoading" size="small" variant="tonal" class="ms-2">
            {{ resources.length }} 个
          </VChip>
        </div>

        <VProgressLinear v-if="detailLoading" indeterminate color="warning" class="mb-4" />

        <template v-else>
          <VRow v-if="resources.length > 0">
            <VCol
              v-for="res in resources"
              :key="res.id"
              cols="12"
              md="6"
            >
              <div class="resource-card">
                <!-- 顶部：用户信息 + 积分徽章 -->
                <div class="d-flex align-center mb-3">
                  <VAvatar
                    v-if="res.user_avatar"
                    :image="res.user_avatar"
                    size="44"
                    class="me-3 resource-avatar"
                  />
                  <VAvatar v-else size="44" color="warning" variant="tonal" class="me-3">
                    <VIcon icon="ri-user-line" size="22" />
                  </VAvatar>
                  <div class="flex-grow-1" style="min-width: 0;">
                    <div class="text-body-1 font-weight-bold text-truncate">
                      {{ res.user_name || '匿名用户' }}
                    </div>
                  </div>
                  <div class="d-flex ga-1 flex-shrink-0">
                    <VChip
                      size="small"
                      :color="res.points === 0 ? 'success' : 'warning'"
                      variant="flat"
                      label
                    >
                      {{ res.points === 0 ? '免费' : res.points + ' 积分' }}
                    </VChip>
                  </div>
                </div>

                <!-- 资源描述 -->
                <div
                  v-if="res.remark || res.title"
                  class="text-body-2 mb-3 resource-desc"
                >
                  {{ res.remark || res.title }}
                </div>

                <!-- 操作按钮 -->
                <div class="d-flex ga-2 mb-4">
                  <VBtn
                    color="warning"
                    variant="flat"
                    :loading="unlockingMap[res.id]"
                    @click.stop="saveResource(res)"
                  >
                    <VIcon icon="ri-download-cloud-line" size="18" class="me-1" />
                    转存
                  </VBtn>
                  <VBtn
                    variant="outlined"
                    :loading="viewingMap[res.id]"
                    @click.stop="viewResource(res)"
                  >
                    <VIcon icon="ri-eye-line" size="18" class="me-1" />
                    查看
                  </VBtn>
                </div>

                <!-- 媒体信息（分组展示） -->
                <div class="resource-meta">
                  <span v-if="getResolutionText(res)" class="meta-group">
                    <VIcon icon="ri-film-line" size="16" color="warning" />
                    <span>{{ getResolutionText(res) }}</span>
                  </span>
                  <span v-if="getSubtitleText(res)" class="meta-group">
                    <VIcon icon="ri-global-line" size="16" color="warning" />
                    <span>{{ getSubtitleText(res) }}</span>
                  </span>
                  <span v-if="getSourceText(res)" class="meta-group">
                    <VIcon icon="ri-disc-line" size="16" color="warning" />
                    <span>{{ getSourceText(res) }}</span>
                  </span>
                  <span v-if="res.size" class="meta-group">
                    <VIcon icon="ri-file-copy-line" size="16" color="warning" />
                    <span>{{ res.size }}</span>
                  </span>
                  <span v-if="res.unlocked_count" class="meta-group">
                    <VIcon icon="ri-group-line" size="16" />
                    <span class="text-medium-emphasis">{{ res.unlocked_count }}</span>
                  </span>
                </div>
              </div>
            </VCol>
          </VRow>

          <div v-else class="text-center pa-10">
            <VIcon icon="ri-inbox-line" size="56" color="rgba(255,255,255,0.15)" class="mb-3" />
            <div class="text-body-1 text-medium-emphasis">
              暂无 115 网盘资源
            </div>
          </div>
        </template>
      </div>
    </template>
    </template>
  </div>
</template>

<style lang="scss" scoped>
// ============ 搜索卡片 ============
.hdhive-card {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  overflow: hidden;
  transition: transform 0.2s, box-shadow 0.2s;
  cursor: pointer;

  &:hover {
    transform: translateY(-2px);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.3);
  }
}

.hdhive-poster-wrap {
  position: relative;
  aspect-ratio: 2/3;
  overflow: hidden;
}

.hdhive-poster {
  width: 100%;
  height: 100%;
}

.hdhive-poster-empty {
  width: 100%;
  height: 100%;
  background: rgba(var(--v-theme-surface-variant), 0.3);
}

.hdhive-rating {
  position: absolute;
  top: 8px;
  right: 8px;
  background: rgba(0, 0, 0, 0.75);
  border-radius: 6px;
  padding: 2px 8px;
  display: flex;
  align-items: center;
  font-size: 12px;
  font-weight: 600;
  color: #fff;
}

.hdhive-type-badge {
  position: absolute;
  top: 8px;
  left: 8px;
}

.hdhive-overview {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  overflow: hidden;
}

// ============ 详情视图 ============
.detail-hero {
  position: relative;
  overflow: hidden;
  border-radius: 12px;
  min-height: 320px;
}

.detail-backdrop {
  position: absolute;
  inset: 0;
  z-index: 0;

  :deep(.v-img) {
    width: 100%;
    height: 100%;
  }

  :deep(.v-img__img) {
    object-fit: cover;
    width: 100%;
    height: 100%;
  }
}

.detail-backdrop-empty {
  width: 100%;
  height: 100%;
  background: linear-gradient(135deg, rgba(var(--v-theme-surface), 0.9), rgba(var(--v-theme-surface), 0.7));
}

.detail-backdrop-overlay {
  position: absolute;
  inset: 0;
  background: linear-gradient(
    to right,
    rgba(var(--v-theme-surface), 0.95) 0%,
    rgba(var(--v-theme-surface), 0.85) 40%,
    rgba(var(--v-theme-surface), 0.55) 100%
  );
}

.detail-content {
  position: relative;
  z-index: 1;
}

.detail-poster {
  align-self: flex-start;
}

// ============ 资源卡片 ============
.resource-card {
  background: rgb(var(--v-theme-surface));
  border: 1px solid rgba(var(--v-border-color), 0.15);
  border-left: 3px solid rgb(var(--v-theme-warning));
  border-radius: 12px;
  padding: 20px;
  transition: all 0.2s ease;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.06);
  display: flex;
  flex-direction: column;
  height: 100%;

  &:hover {
    border-color: rgba(var(--v-theme-warning), 0.4);
    transform: translateY(-1px);
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.1);
  }
}

.resource-avatar {
  border: 2px solid rgba(var(--v-theme-warning), 0.4);
}

.resource-desc {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
  line-clamp: 3;
  overflow: hidden;
  color: rgba(var(--v-theme-on-surface), 0.65);
  line-height: 1.6;
  flex-grow: 1;
}

.resource-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 20px;
  padding-top: 16px;
  border-top: 1px solid rgba(var(--v-border-color), 0.1);
}

.meta-group {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 14px;
  color: rgba(var(--v-theme-on-surface), 0.65);
  white-space: nowrap;
}
</style>

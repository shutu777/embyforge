<script setup>
import { ref, onMounted, onBeforeUnmount, computed } from 'vue'
import { useClipboard } from '@vueuse/core'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'

const snackbar = useSnackbar()
const { copy: clipboardCopy } = useClipboard({ legacy: true })
const cacheStatus = ref(null)
const loadingStatus = ref(false)

// 同步状态
const syncing = ref(false)
const syncResult = ref(null)

// SSE 进度状态
const syncProgress = ref(null)
let eventSource = null

const hasCache = computed(() => cacheStatus.value && cacheStatus.value.total_items > 0)
const isIndeterminate = computed(() => syncProgress.value && syncProgress.value.total === 0)

// Webhook 配置
const pendingEvents = ref(0)
const webhookUrl = ref('/api/webhook/emby')

function copyWebhookUrl() {
  clipboardCopy(webhookUrl.value).then(() => {
    snackbar.success('Webhook 地址已复制到剪贴板')
  }).catch(() => {
    snackbar.error('复制失败，请手动复制')
  })
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

// 毫秒转 X分Y秒
function formatDuration(ms) {
  if (!ms || ms <= 0) return '-'
  const totalSec = Math.round(ms / 1000)
  const min = Math.floor(totalSec / 60)
  const sec = totalSec % 60
  if (min === 0) return `${sec} 秒`
  return `${min} 分 ${sec} 秒`
}

async function fetchCacheStatus() {
  loadingStatus.value = true
  try {
    const { data } = await api.get('/cache/status')
    cacheStatus.value = data.data
    pendingEvents.value = data.pending_events || 0
  } catch (e) {
    console.error('获取缓存状态失败', e)
  } finally {
    loadingStatus.value = false
  }
}

function closeSSE() {
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
}

function connectSSE() {
  closeSSE()
  const token = localStorage.getItem('token')
  if (!token) {
    snackbar.error('认证令牌缺失，请重新登录')
    syncing.value = false
    syncProgress.value = null
    return
  }

  syncing.value = true
  syncResult.value = null
  if (!syncProgress.value) {
    syncProgress.value = { processed: 0, total: 0, percent: 0, phase: 'media' }
  }

  const baseURL = import.meta.env.VITE_API_BASE_URL || '/api'
  eventSource = new EventSource(`${baseURL}/cache/sync/stream?token=${encodeURIComponent(token)}&fullSync=true`)

  eventSource.addEventListener('progress', (e) => {
    try {
      const data = JSON.parse(e.data)
      syncProgress.value = {
        processed: data.processed,
        total: data.total,
        percent: data.percent || 0,
        phase: data.phase,
      }
    } catch (err) {
      console.error('解析进度事件失败', err)
    }
  })

  eventSource.addEventListener('done', async (e) => {
    closeSSE()
    try {
      syncResult.value = JSON.parse(e.data)
      snackbar.success('媒体库同步完成')
    } catch {
      syncResult.value = { error: '解析同步结果失败' }
    }
    syncing.value = false
    syncProgress.value = null
    await fetchCacheStatus()
  })

  eventSource.addEventListener('error', (e) => {
    closeSSE()
    let msg = '同步失败'
    if (e.data) {
      try { msg = JSON.parse(e.data).message || msg } catch {}
    }
    syncResult.value = { error: msg }
    snackbar.error(msg)
    syncing.value = false
    syncProgress.value = null
  })

  eventSource.onerror = () => {
    if (syncing.value && eventSource && eventSource.readyState === EventSource.CLOSED) {
      closeSSE()
      syncResult.value = { error: 'SSE 连接异常断开' }
      snackbar.error('SSE 连接异常断开')
      syncing.value = false
      syncProgress.value = null
    }
  }
}

function syncMedia() {
  connectSSE()
}

async function checkActiveSync() {
  try {
    const { data } = await api.get('/cache/sync/status')
    if (data.syncing) {
      if (data.progress) {
        const p = data.progress
        syncProgress.value = {
          processed: p.processed,
          total: p.total,
          percent: p.total > 0 ? (p.processed / p.total) * 100 : 0,
          phase: p.phase,
        }
      }
      connectSSE()
    }
  } catch (e) {
    console.error('检查同步状态失败', e)
  }
}

onMounted(async () => {
  await fetchCacheStatus()
  await checkActiveSync()
})

onBeforeUnmount(closeSSE)
</script>

<template>
  <div class="media-scan">
    <!-- 页面标题和说明 -->
    <div class="mb-6">
      <h1 class="text-h4 font-weight-bold mb-2">媒体库同步</h1>
      <p class="text-body-1 text-medium-emphasis">
        从 Emby 服务器同步媒体库数据到本地缓存，支持全量同步和 Webhook 自动增量同步
      </p>
    </div>

    <!-- 加载中 -->
    <div v-if="loadingStatus" class="d-flex justify-center align-center" style="min-height: 300px;">
      <VProgressCircular indeterminate color="primary" size="48" />
    </div>

    <template v-else>
      <!-- 第一行：缓存统计卡片 -->
      <VRow class="mb-4 match-height">
        <VCol cols="6" sm="4">
          <VCard class="stat-card">
            <VCardText class="d-flex align-center justify-space-between h-100 pa-5 stat-card-text">
              <div class="stat-text-wrap">
                <div class="text-body-2 text-medium-emphasis mb-1">媒体条目</div>
                <div class="font-weight-bold stat-number">
                  {{ hasCache ? cacheStatus.total_items.toLocaleString() : '0' }}
                </div>
              </div>
              <div class="stat-icon" style="background: #6366f118;">
                <VIcon icon="ri-film-fill" color="#6366f1" size="24" />
              </div>
            </VCardText>
          </VCard>
        </VCol>
        <VCol cols="6" sm="4">
          <VCard class="stat-card">
            <VCardText class="d-flex align-center justify-space-between h-100 pa-5 stat-card-text">
              <div class="stat-text-wrap">
                <div class="text-body-2 text-medium-emphasis mb-1">季缓存</div>
                <div class="font-weight-bold stat-number">
                  {{ hasCache ? cacheStatus.total_seasons.toLocaleString() : '0' }}
                </div>
              </div>
              <div class="stat-icon" style="background: #06b6d418;">
                <VIcon icon="ri-folder-video-fill" color="#06b6d4" size="24" />
              </div>
            </VCardText>
          </VCard>
        </VCol>
        <VCol cols="12" sm="4">
          <VCard class="stat-card">
            <VCardText class="d-flex align-center justify-space-between h-100 pa-5 stat-card-text">
              <div class="stat-text-wrap">
                <div class="text-body-2 text-medium-emphasis mb-1">最后同步</div>
                <div class="font-weight-bold stat-number">
                  {{ hasCache ? formatTime(cacheStatus.last_sync_at) : '-' }}
                </div>
              </div>
              <div class="stat-icon" style="background: #f59e0b18;">
                <VIcon icon="ri-time-fill" color="#f59e0b" size="24" />
              </div>
            </VCardText>
          </VCard>
        </VCol>
      </VRow>

      <!-- 第二行：同步操作 -->
      <VCard variant="flat" class="content-card mb-7" data-no-hover>
        <VCardText class="pa-5">
          <div class="d-flex align-center mb-4">
            <VAvatar color="primary" variant="tonal" size="42" rounded="lg" class="me-3">
              <VIcon icon="ri-refresh-line" size="22" />
            </VAvatar>
            <div>
              <div class="text-body-1 font-weight-semibold">全量同步媒体库</div>
              <div class="text-body-2 text-medium-emphasis">
                从 Emby 服务器拉取完整媒体库信息并缓存到本地，用于后续分析检测
              </div>
            </div>
          </div>

          <VAlert type="warning" variant="tonal" density="compact" class="mb-4">
            同步前请确认已使用最新的目录树生成 strm 文件，否则缓存数据可能不准确
          </VAlert>

          <VBtn
            color="primary"
            :disabled="syncing"
            :loading="syncing && !syncProgress"
            size="default"
            @click="syncMedia"
          >
            <VIcon icon="ri-loop-left-line" class="me-1" />
            {{ syncing ? '同步中...' : '开始全量同步' }}
          </VBtn>

          <div class="text-caption text-medium-emphasis mt-2">
            ⚠ 将清空本地缓存并从 Emby 重新拉取所有数据
          </div>

          <!-- 进度条区域 -->
          <div v-if="syncing && syncProgress" class="sync-progress mt-5">
            <div class="d-flex align-center mb-2">
              <VProgressCircular
                indeterminate
                color="primary"
                size="16"
                width="2"
                class="me-2"
              />
              <span class="text-body-2 font-weight-medium">
                正在{{ syncProgress.phase === 'season' ? '重建季缓存' : '同步媒体库' }}
              </span>
            </div>

            <VProgressLinear
              :model-value="isIndeterminate ? undefined : syncProgress.percent"
              :indeterminate="isIndeterminate"
              color="primary"
              height="10"
              rounded
              class="mb-2"
            />

            <div v-if="!isIndeterminate" class="d-flex justify-space-between">
              <span class="text-caption text-medium-emphasis">
                已处理 {{ syncProgress.processed.toLocaleString() }} / {{ syncProgress.total.toLocaleString() }}
              </span>
              <span class="text-caption font-weight-bold" style="color: #6366f1;">
                {{ syncProgress.percent.toFixed(1) }}%
              </span>
            </div>
          </div>

          <!-- 同步结果 -->
          <VCard
            v-if="syncResult && !syncResult.error"
            variant="tonal"
            color="success"
            class="mt-4"
          >
            <VCardText class="d-flex align-center pa-4">
              <VAvatar color="success" variant="tonal" size="38" rounded="lg" class="me-3">
                <VIcon icon="ri-check-line" size="20" />
              </VAvatar>
              <div>
                <div class="text-body-2 font-weight-semibold">同步完成</div>
                <div class="text-caption text-medium-emphasis">
                  共 {{ syncResult.total_items?.toLocaleString() }} 个媒体条目，{{ syncResult.total_seasons?.toLocaleString() }} 个季，耗时 {{ formatDuration(syncResult.elapsed_ms) }}
                </div>
              </div>
            </VCardText>
          </VCard>

          <VCard
            v-if="syncResult && syncResult.error"
            variant="tonal"
            color="error"
            class="mt-4"
          >
            <VCardText class="d-flex align-center pa-4">
              <VAvatar color="error" variant="tonal" size="38" rounded="lg" class="me-3">
                <VIcon icon="ri-error-warning-line" size="20" />
              </VAvatar>
              <div>
                <div class="text-body-2 font-weight-semibold">同步失败</div>
                <div class="text-caption text-medium-emphasis">{{ syncResult.error }}</div>
              </div>
            </VCardText>
          </VCard>
        </VCardText>
      </VCard>

      <!-- 第三行：Emby Webhook 配置引导 -->
      <VCard variant="flat" class="content-card" data-no-hover>
        <VCardText class="pa-5">
          <div class="d-flex align-center justify-space-between mb-4">
            <div class="d-flex align-center">
              <VAvatar color="success" variant="tonal" size="42" rounded="lg" class="me-3">
                <VIcon icon="ri-webhook-line" size="22" />
              </VAvatar>
              <div>
                <div class="text-body-1 font-weight-semibold">Emby Webhook 自动同步</div>
                <div class="text-body-2 text-medium-emphasis">
                  配置 Webhook 后，媒体库变更将自动推送到本地缓存
                </div>
              </div>
            </div>
            <VChip v-if="pendingEvents > 0" color="warning" variant="tonal" size="small">
              <VIcon icon="ri-loader-4-line" size="14" class="me-1" />
              {{ pendingEvents }} 待处理
            </VChip>
          </div>

          <!-- Webhook 地址区域 -->
          <VCard variant="tonal" color="success" class="mb-4" flat data-no-hover>
            <VCardText class="pa-4">
              <div class="text-body-2 font-weight-semibold mb-2">
                <VIcon icon="ri-check-line" size="18" class="me-1" />
                Webhook 地址
              </div>
              <div class="d-flex align-center">
                <VTextField
                  :model-value="webhookUrl"
                  readonly
                  density="compact"
                  variant="outlined"
                  hide-details
                  class="me-2"
                />
                <VBtn
                  color="success"
                  variant="tonal"
                  @click="copyWebhookUrl"
                >
                  <VIcon icon="ri-file-copy-line" class="me-1" />
                  复制
                </VBtn>
              </div>
            </VCardText>
          </VCard>

          <div class="text-body-2 font-weight-medium mb-2">
            <VIcon icon="ri-question-line" size="18" class="me-1" />
            如何在 Emby 中配置 Webhook？
          </div>
          <ol class="text-body-2 ps-4 mb-4" style="line-height: 1.9;">
            <li>打开 Emby 服务器管理面板</li>
            <li>进入 <strong>服务器 → 通知</strong>（或安装 Webhooks 插件）</li>
            <li>添加一个新的 Webhook 通知</li>
            <li>将上方的 Webhook 地址填入 URL 字段</li>
            <li>Request Content Type 选择 <strong>application/json</strong></li>
            <li>勾选 <strong>媒体库全部事件</strong></li>
            <li>保存配置即可，后续媒体库变更会自动推送到 EmbyForge</li>
          </ol>
          <VAlert type="warning" variant="tonal" density="compact">
            <div class="text-body-2">
              如果 EmbyForge 部署在 Docker 中，请确保 Emby 服务器能访问到此地址。
              使用 Docker 内部网络时，地址可能需要替换为容器名或内网 IP。
            </div>
          </VAlert>
        </VCardText>
      </VCard>
    </template>
  </div>
</template>

<style lang="scss" scoped>
// 页面特有样式（通用样式已提取到 page-common.scss）
.sync-progress {
  padding: 16px;
  border-radius: 12px;
  background: rgba(var(--v-theme-primary), 0.04);
  border: 1px solid rgba(var(--v-theme-primary), 0.12);
}

@media (max-width: 599.98px) {
  .sync-progress {
    padding: 12px;
  }
}
</style>

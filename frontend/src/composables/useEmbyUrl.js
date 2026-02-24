import { ref } from 'vue'
import api from '@/utils/api'

// 全局单例：从后端获取的 Emby 基础 URL
const embyBaseUrl = ref('')
const embyServerId = ref('')
const embyApiKey = ref('')
const detected = ref(false)

/**
 * 从后端获取经过可达性探测的 Emby 地址
 * 后端已完成内网/外网探测，直接使用返回的 base_url
 */
async function detectEmbyUrl() {
  if (detected.value) return

  try {
    const { data } = await api.get('/emby-config/server-info')
    const info = data.data
    if (!info) return

    embyBaseUrl.value = info.base_url || ''
    embyServerId.value = info.server_id || ''
    embyApiKey.value = info.api_key || ''
    detected.value = true
  } catch (e) {
    console.error('获取 Emby 地址失败', e)
  }
}

/**
 * 构建 Emby 图片 URL
 */
function embyImageUrl(itemId, maxHeight = 300) {
  if (!itemId || !embyBaseUrl.value) return ''
  return `${embyBaseUrl.value}/emby/Items/${itemId}/Images/Primary?maxHeight=${maxHeight}&api_key=${embyApiKey.value}`
}

/**
 * 构建 Emby Web 跳转 URL
 */
function embyWebUrl(itemId) {
  if (!itemId || !embyBaseUrl.value || !embyServerId.value) return ''
  return `${embyBaseUrl.value}/web/index.html#!/item?id=${itemId}&serverId=${embyServerId.value}`
}

export function useEmbyUrl() {
  return {
    embyBaseUrl,
    embyServerId,
    embyApiKey,
    detected,
    detectEmbyUrl,
    embyImageUrl,
    embyWebUrl,
  }
}

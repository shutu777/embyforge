<script setup>
import { ref, onMounted } from 'vue'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'

const snackbar = useSnackbar()

// 配置列表
const configs = ref([])
const loading = ref(false)

// 保存状态（按 key 跟踪）
const savingKeys = ref({})

// 密码可见性切换（按 key 跟踪）
const visibleKeys = ref({})

// 敏感字段列表（使用密码输入框）
const sensitiveKeys = ['tmdb_api_key']

function isSensitive(key) {
  return sensitiveKeys.includes(key)
}

// 页面加载时获取所有配置
onMounted(async () => {
  await fetchConfigs()
})

async function fetchConfigs() {
  loading.value = true
  try {
    const { data } = await api.get('/system-config')
    // 过滤掉 symedia 相关配置项（在 symedia-config 页面单独管理）
    const symediaKeys = ['symedia_url', 'symedia_auth_token']
    configs.value = (data.data || [])
      .filter(c => !symediaKeys.includes(c.key))
      .map(c => ({ ...c, editValue: c.value }))
  } catch (e) {
    snackbar.error('获取配置失败')
  } finally {
    loading.value = false
  }
}

// 保存单个配置项
async function saveConfig(config) {
  savingKeys.value[config.key] = true
  try {
    const { data } = await api.put(`/system-config/${config.key}`, {
      value: config.editValue,
    })
    // 更新本地数据
    config.value = config.editValue
    config.updated_at = data.data?.updated_at || config.updated_at
    snackbar.success(data.message || '配置更新成功')
  } catch (e) {
    snackbar.error(e.response?.data?.message || '保存失败')
  } finally {
    savingKeys.value[config.key] = false
  }
}
</script>

<template>
  <VCard title="系统配置" data-no-hover>
    <VCardText>
      <!-- 加载状态 -->
      <div v-if="loading" class="d-flex justify-center py-6">
        <VProgressCircular indeterminate color="primary" />
      </div>

      <!-- 配置列表 -->
      <VRow v-else>
        <VCol
          v-for="config in configs"
          :key="config.key"
          cols="12"
        >
          <VTextField
            v-model="config.editValue"
            :label="config.description || config.key"
            :placeholder="`输入 ${config.key}`"
            :type="isSensitive(config.key) && !visibleKeys[config.key] ? 'password' : 'text'"
            :append-inner-icon="isSensitive(config.key) ? (visibleKeys[config.key] ? 'ri-eye-off-line' : 'ri-eye-line') : undefined"
            persistent-hint
            :hint="`键名: ${config.key}`"
            @click:append-inner="visibleKeys[config.key] = !visibleKeys[config.key]"
          >
            <template #append>
              <VBtn
                color="primary"
                size="small"
                :loading="savingKeys[config.key]"
                @click="saveConfig(config)"
              >
                保存
              </VBtn>
            </template>
          </VTextField>
        </VCol>

        <!-- 空状态 -->
        <VCol v-if="!loading && configs.length === 0" cols="12">
          <VAlert type="info">
            暂无配置项
          </VAlert>
        </VCol>
      </VRow>
    </VCardText>
  </VCard>
</template>

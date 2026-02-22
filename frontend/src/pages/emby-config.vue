<script setup>
import { ref, onMounted } from 'vue'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'

const snackbar = useSnackbar()

// 表单数据
const host = ref('')
const port = ref(8096)
const apiKey = ref('')

// 状态
const saving = ref(false)
const testing = ref(false)

// 页面加载时获取已保存配置
onMounted(async () => {
  try {
    const { data } = await api.get('/emby-config')
    if (data.data) {
      host.value = data.data.host || ''
      port.value = data.data.port || 8096
      apiKey.value = data.data.api_key || ''
    }
  } catch (e) {
    console.error('获取配置失败', e)
  }
})

// 保存配置
async function saveConfig() {
  saving.value = true
  try {
    const { data } = await api.post('/emby-config', {
      host: host.value,
      port: port.value,
      api_key: apiKey.value,
    })
    snackbar.success(data.message || '配置保存成功')
  } catch (e) {
    snackbar.error(e.response?.data?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

// 测试连接
async function testConnection() {
  testing.value = true
  try {
    const { data } = await api.post('/emby-config/test', {
      host: host.value,
      port: port.value,
      api_key: apiKey.value,
    })
    snackbar.success(`连接成功 - 服务器: ${data.server_name}, 版本: ${data.version}`)
  } catch (e) {
    const errData = e.response?.data
    const msg = errData?.error
      ? `连接失败: ${errData.error}`
      : (errData?.message || '连接失败')
    snackbar.error(msg)
  } finally {
    testing.value = false
  }
}
</script>

<template>
  <VCard title="Emby 配置" data-no-hover>
    <VCardText>
      <VForm @submit.prevent="saveConfig">
        <VRow>
          <VCol cols="12" md="8">
            <VTextField
              v-model="host"
              label="服务器地址"
              placeholder="http://192.168.1.100"
              hint="例如 http://192.168.1.100"
              persistent-hint
            />
          </VCol>
          <VCol cols="12" md="4">
            <VTextField
              v-model.number="port"
              label="端口"
              type="number"
              placeholder="8096"
            />
          </VCol>
          <VCol cols="12">
            <VTextField
              v-model="apiKey"
              label="API Key"
              placeholder="输入 Emby API Key"
              type="password"
            />
          </VCol>
          <VCol cols="12">
            <div class="d-flex flex-wrap gap-3">
              <VBtn
                type="submit"
                color="primary"
                :loading="saving"
              >
                保存配置
              </VBtn>
              <VBtn
                color="secondary"
                variant="outlined"
                :loading="testing"
                @click="testConnection"
              >
                测试连接
              </VBtn>
            </div>
          </VCol>
        </VRow>
      </VForm>
    </VCardText>
  </VCard>
</template>

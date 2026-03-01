<script setup>
import { ref, onMounted } from 'vue'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'

const snackbar = useSnackbar()

// 表单数据
const host = ref('')
const port = ref(8096)
const apiKey = ref('')
const username = ref('')
const password = ref('')
const hasPassword = ref(false)
const externalUrl = ref('')

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
      username.value = data.data.username || ''
      hasPassword.value = data.data.has_password || false
      externalUrl.value = data.data.external_url || ''
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
      username: username.value,
      password: password.value,
      external_url: externalUrl.value,
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
      username: username.value,
      password: password.value,
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
  <div>
    <!-- 页面标题和说明 -->
    <div class="mb-6">
      <h1 class="text-h4 font-weight-bold mb-2">Emby 配置</h1>
      <p class="text-body-1 text-medium-emphasis">
        配置 Emby 服务器连接信息和用户认证，用于媒体库数据同步和管理操作
      </p>
    </div>

    <VCard variant="flat" class="content-card" data-no-hover>
      <VCardText class="pa-6">
        <div class="d-flex align-center mb-5">
          <VAvatar color="primary" variant="tonal" size="42" rounded="lg" class="me-3">
            <VIcon icon="ri-server-line" size="22" />
          </VAvatar>
          <div>
            <div class="text-body-1 font-weight-semibold">Emby 服务器设置</div>
            <div class="text-body-2 text-medium-emphasis">配置服务器地址、API Key 和用户认证信息</div>
          </div>
        </div>

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
                v-model="externalUrl"
                label="外网访问地址（可选）"
                placeholder="https://emby.example.com"
                hint="通过反向代理从外网访问时填写，留空则使用内网地址。用于封面图片加载和 Emby Web 跳转"
                persistent-hint
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
              <VDivider class="mb-2" />
              <div class="text-subtitle-2 text-medium-emphasis mb-2">
                用户认证（删除操作需要）
              </div>
            </VCol>
            <VCol cols="12" md="6">
              <VTextField
                v-model="username"
                label="Emby 用户名"
                placeholder="输入 Emby 用户名"
                hint="用于删除媒体时的用户认证"
                persistent-hint
              />
            </VCol>
            <VCol cols="12" md="6">
              <VTextField
                v-model="password"
                label="Emby 密码"
                :placeholder="hasPassword ? '已保存，留空则不修改' : '输入 Emby 密码'"
                type="password"
                :hint="hasPassword ? '已保存密码，留空则保持不变' : '用于删除媒体时的用户认证'"
                persistent-hint
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
  </div>
</template>

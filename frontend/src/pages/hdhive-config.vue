<script setup>
import { ref, onMounted } from 'vue'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'

const snackbar = useSnackbar()

// 表单数据
const username = ref('')
const password = ref('')
const token = ref('')
const cookie = ref('')

// 状态
const saving = ref(false)
const loggingIn = ref(false)
const pageLoading = ref(false)

// 密码可见性
const showPassword = ref(false)

// 页面加载时获取已保存配置
onMounted(async () => {
  pageLoading.value = true
  try {
    const { data } = await api.get('/system-config')
    if (data.data) {
      for (const config of data.data) {
        switch (config.key) {
          case 'hdhive_username':
            username.value = config.value || ''
            break
          case 'hdhive_password':
            password.value = config.value || ''
            break
          case 'hdhive_token':
            token.value = config.value || ''
            break
          case 'hdhive_cookie':
            cookie.value = config.value || ''
            break
        }
      }
    }
  } catch (e) {
    console.error('获取配置失败', e)
  } finally {
    pageLoading.value = false
  }
})

// 保存配置
async function saveConfig() {
  if (!username.value.trim()) {
    snackbar.error('请输入 HDHive 账号')
    return
  }
  saving.value = true
  try {
    await api.put('/system-config/hdhive_username', { value: username.value.trim() })
    if (password.value.trim()) {
      await api.put('/system-config/hdhive_password', { value: password.value.trim() })
    }
    snackbar.success('HDHive 配置保存成功')
  } catch (e) {
    snackbar.error(e.response?.data?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

// 登录 HDHive
async function loginHDHive() {
  loggingIn.value = true
  try {
    const { data } = await api.post('/hdhive/login')
    snackbar.success(data.message || 'HDHive 登录成功')
    // 更新 token 和 cookie 显示
    if (data.data) {
      token.value = data.data.token || ''
      cookie.value = data.data.cookie || ''
    }
  } catch (e) {
    snackbar.error(e.response?.data?.message || 'HDHive 登录失败')
  } finally {
    loggingIn.value = false
  }
}
</script>

<template>
  <div>
    <!-- 页面标题和说明 -->
    <div class="mb-6">
      <h1 class="text-h4 font-weight-bold mb-2">HDHive 配置</h1>
      <p class="text-body-1 text-medium-emphasis">
        配置 HDHive 账号密码，登录后自动缓存 Token 和 Cookie，用于 HDHive 搜索功能
      </p>
    </div>

    <VCard variant="flat" class="content-card" data-no-hover>
      <VCardText class="pa-5">
        <div class="d-flex align-center mb-5">
          <VAvatar color="warning" variant="tonal" size="42" rounded="lg" class="me-3">
            <VIcon icon="ri-fire-line" size="22" />
          </VAvatar>
          <div>
            <div class="text-body-1 font-weight-semibold">HDHive 账号设置</div>
            <div class="text-body-2 text-medium-emphasis">配置登录凭证，用于资源搜索功能</div>
          </div>
        </div>

        <VForm @submit.prevent="saveConfig">
          <VRow>
            <!-- 账号 -->
            <VCol cols="12" md="6">
              <VTextField
                v-model="username"
                label="HDHive 账号"
                placeholder="输入邮箱或用户名"
                prepend-inner-icon="ri-user-line"
              />
            </VCol>
            <!-- 密码 -->
            <VCol cols="12" md="6">
              <VTextField
                v-model="password"
                label="HDHive 密码"
                placeholder="输入密码"
                :type="showPassword ? 'text' : 'password'"
                prepend-inner-icon="ri-lock-line"
                :append-inner-icon="showPassword ? 'ri-eye-off-line' : 'ri-eye-line'"
                @click:append-inner="showPassword = !showPassword"
              />
            </VCol>

            <!-- Token（只读） -->
            <VCol cols="12">
              <VTextField
                v-model="token"
                label="Token（登录后自动缓存）"
                readonly
                prepend-inner-icon="ri-key-line"
                hint="登录成功后自动填充，无需手动编辑"
                persistent-hint
              />
            </VCol>

            <!-- Cookie（只读） -->
            <VCol cols="12">
              <VTextField
                v-model="cookie"
                label="Cookie（登录后自动缓存）"
                readonly
                prepend-inner-icon="ri-cookie-line"
                hint="登录成功后自动填充，无需手动编辑"
                persistent-hint
              />
            </VCol>

            <!-- 操作按钮 -->
            <VCol cols="12">
              <div class="d-flex flex-wrap gap-3">
                <VBtn
                  type="submit"
                  color="primary"
                  :loading="saving"
                  prepend-icon="ri-save-line"
                >
                  保存配置
                </VBtn>
                <VBtn
                  color="success"
                  variant="outlined"
                  :loading="loggingIn"
                  prepend-icon="ri-login-box-line"
                  @click="loginHDHive"
                >
                  登录 HDHive
                </VBtn>
              </div>
            </VCol>
          </VRow>
        </VForm>
      </VCardText>
    </VCard>

    <!-- 状态提示 -->
    <VCard v-if="token" variant="flat" class="content-card mt-4" data-no-hover>
      <VCardText class="pa-5">
        <div class="d-flex align-center gap-2">
          <VIcon icon="ri-checkbox-circle-line" color="success" />
          <span class="text-body-1">HDHive 已登录，Token 有效</span>
        </div>
      </VCardText>
    </VCard>
    <VCard v-else variant="flat" class="content-card mt-4" data-no-hover>
      <VCardText class="pa-5">
        <div class="d-flex align-center gap-2">
          <VIcon icon="ri-error-warning-line" color="warning" />
          <span class="text-body-1">HDHive 未登录，请先保存配置后点击登录</span>
        </div>
      </VCardText>
    </VCard>
  </div>
</template>

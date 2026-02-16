<script setup>
import { useTheme } from 'vuetify'
import { useRouter } from 'vue-router'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'
import logo from '@images/logo.svg?raw'
import authV1MaskDark from '@images/pages/auth-v1-mask-dark.png'
import authV1MaskLight from '@images/pages/auth-v1-mask-light.png'
import authV1Tree2 from '@images/pages/auth-v1-tree-2.png'
import authV1Tree from '@images/pages/auth-v1-tree.png'

const router = useRouter()

const form = ref({
  username: '',
  password: '',
})

const vuetifyTheme = useTheme()

const authThemeMask = computed(() => {
  return vuetifyTheme.global.name.value === 'light' ? authV1MaskLight : authV1MaskDark
})

const isPasswordVisible = ref(false)
const isLoading = ref(false)
const errorMessage = ref('')
const snackbar = useSnackbar()

// 登录处理：调用后端 API，成功后存储 JWT 并跳转首页
async function handleLogin() {
  errorMessage.value = ''
  isLoading.value = true
  try {
    const { data } = await api.post('/auth/login', {
      username: form.value.username,
      password: form.value.password,
    })

    localStorage.setItem('token', data.token)
    router.push({ name: 'dashboard' })
  }
  catch (err) {
    errorMessage.value = err.response?.data?.message || '登录失败，请检查用户名和密码'
    snackbar.error(errorMessage.value)
  }
  finally {
    isLoading.value = false
  }
}
</script>

<template>
  <!-- eslint-disable vue/no-v-html -->
  <div class="auth-wrapper d-flex align-center justify-center pa-4">
    <VCard
      class="auth-card pa-16 pt-16 rounded-0"
      max-width="500"
    >
      <VCardItem class="justify-center">
        <div class="d-flex align-center gap-3">
          <div
            class="d-flex"
            v-html="logo"
          />
          <h2 class="font-weight-medium text-2xl text-uppercase">
            EmbyForge
          </h2>
        </div>
      </VCardItem>

      <VCardText class="mt-3">
        <VForm @submit.prevent="handleLogin">
          <VRow>
            <!-- 用户名 -->
            <VCol cols="12">
              <VTextField
                v-model="form.username"
                :label="$t('common.username')"
                density="comfortable"
                variant="outlined"
                class="text-lg"
                :disabled="isLoading"
              />
            </VCol>

            <!-- 密码 -->
            <VCol cols="12">
              <VTextField
                v-model="form.password"
                :label="$t('common.password')"
                placeholder="············"
                :type="isPasswordVisible ? 'text' : 'password'"
                autocomplete="current-password"
                :append-inner-icon="isPasswordVisible ? 'ri-eye-off-line' : 'ri-eye-line'"
                density="comfortable"
                variant="outlined"
                class="text-lg"
                :disabled="isLoading"
                @click:append-inner="isPasswordVisible = !isPasswordVisible"
              />

              <!-- 错误提示 -->
              <VAlert
                v-if="errorMessage"
                type="error"
                variant="tonal"
                class="mt-3"
              >
                {{ errorMessage }}
              </VAlert>

              <!-- 登录按钮 -->
              <VBtn
                block
                type="submit"
                size="large"
                class="text-lg mt-6"
                :loading="isLoading"
                :disabled="isLoading"
              >
                {{ $t('common.login') }}
              </VBtn>
            </VCol>
          </VRow>
        </VForm>
      </VCardText>
    </VCard>

    <VImg
      class="auth-footer-start-tree d-none d-md-block"
      :src="authV1Tree"
      :width="250"
    />

    <VImg
      :src="authV1Tree2"
      class="auth-footer-end-tree d-none d-md-block"
      :width="350"
    />

    <VImg
      class="auth-footer-mask d-none d-md-block"
      :src="authThemeMask"
    />
  </div>
</template>

<style lang="scss" scoped>
@use "@core/scss/template/pages/page-auth";

.auth-card {
  border-radius: 0 !important;
  width: 100% !important;
}

:deep(.v-field) {
  min-height: 50px !important;
  font-size: 15px !important;
}

:deep(.v-field__input) {
  min-height: 50px !important;
  padding-top: 14px !important;
  padding-bottom: 14px !important;
  font-size: 15px !important;
}

:deep(.v-btn) {
  min-height: 48px !important;
  font-size: 16px !important;
}
</style>

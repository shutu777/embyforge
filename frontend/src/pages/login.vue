<script setup>
import { useTheme } from 'vuetify'
import { useRouter } from 'vue-router'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'
const logo = `<svg width="2.5em" height="2.5em" viewBox="0 0 32 32" xmlns="http://www.w3.org/2000/svg"><defs><linearGradient id="lgBg" x1="0" y1="0" x2="32" y2="32" gradientUnits="userSpaceOnUse"><stop offset="0%" stop-color="#93c5fd"/><stop offset="30%" stop-color="#3b82f6"/><stop offset="70%" stop-color="#6366f1"/><stop offset="100%" stop-color="#4338ca"/></linearGradient></defs><rect width="32" height="32" rx="7" fill="url(#lgBg)"/><g transform="translate(4,4)"><path d="M11.041 0c-.007 0-1.456 1.43-3.219 3.176L4.615 6.352l.512.513.512.512-2.819 2.791L0 12.961l1.83 1.848c1.006 1.016 2.438 2.46 3.182 3.209l1.351 1.359.508-.496c.28-.273.515-.498.524-.498.008 0 1.266 1.264 2.794 2.808L12.97 24l.187-.182c.23-.225 5.007-4.95 5.717-5.656l.52-.516-.502-.513c-.276-.282-.5-.52-.496-.53.003-.009 1.264-1.26 2.802-2.783 1.538-1.522 2.8-2.776 2.803-2.785.005-.012-3.617-3.684-6.107-6.193L17.65 4.6l-.505.505c-.279.278-.517.501-.53.497-.013-.005-1.27-1.267-2.793-2.805A449.655 449.655 0 0011.041 0zM9.223 7.367c.091.038 7.951 4.608 7.957 4.627.003.013-1.781 1.056-3.965 2.32a999.898 999.898 0 01-3.996 2.307c-.019.006-.026-1.266-.026-4.629 0-3.7.007-4.634.03-4.625Z" fill="white"/></g></svg>`
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

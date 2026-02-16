<script setup>
import { ref, onMounted } from 'vue'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'
import defaultAvatar from '@images/avatars/avatar-1.png'

const snackbar = useSnackbar()

const loading = ref(true)
const profile = ref({ username: '', avatar: '' })

// 头像相关
const avatarPreview = ref('')
const avatarFile = ref(null)
const uploadLoading = ref(false)

// 用户名相关
const newUsername = ref('')
const usernameLoading = ref(false)

// 密码相关
const passwordForm = ref({ old_password: '', new_password: '', confirm_password: '' })
const passwordLoading = ref(false)
const showOldPwd = ref(false)
const showNewPwd = ref(false)
const showConfirmPwd = ref(false)

// 获取个人信息
async function fetchProfile() {
  loading.value = true
  try {
    const res = await api.get('/profile')
    profile.value = res.data.data
    newUsername.value = profile.value.username
    avatarPreview.value = profile.value.avatar || ''
  } catch (e) {
    snackbar.error('获取个人信息失败')
  } finally {
    loading.value = false
  }
}

onMounted(fetchProfile)

// 选择头像文件
function onAvatarChange(e) {
  const file = e.target.files?.[0]
  if (!file) return
  if (file.size > 2 * 1024 * 1024) {
    snackbar.error('头像文件不能超过 2MB')
    return
  }
  avatarFile.value = file
  avatarPreview.value = URL.createObjectURL(file)
}

// 上传头像
async function uploadAvatar() {
  if (!avatarFile.value) {
    snackbar.error('请先选择头像文件')
    return
  }
  uploadLoading.value = true
  try {
    const formData = new FormData()
    formData.append('avatar', avatarFile.value)
    const res = await api.post('/profile/avatar', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
    profile.value.avatar = res.data.data.avatar
    avatarFile.value = null
    snackbar.success('头像上传成功')
  } catch (e) {
    snackbar.error(e.response?.data?.message || '头像上传失败')
  } finally {
    uploadLoading.value = false
  }
}

// 修改用户名
async function changeUsername() {
  if (!newUsername.value || newUsername.value.length < 2) {
    snackbar.error('用户名至少 2 个字符')
    return
  }
  usernameLoading.value = true
  try {
    await api.put('/profile/username', { username: newUsername.value })
    profile.value.username = newUsername.value
    snackbar.success('用户名修改成功')
  } catch (e) {
    snackbar.error(e.response?.data?.message || '用户名修改失败')
  } finally {
    usernameLoading.value = false
  }
}

// 修改密码
async function changePassword() {
  if (!passwordForm.value.old_password || !passwordForm.value.new_password) {
    snackbar.error('请填写完整的密码信息')
    return
  }
  if (passwordForm.value.new_password.length < 4) {
    snackbar.error('新密码至少 4 个字符')
    return
  }
  if (passwordForm.value.new_password !== passwordForm.value.confirm_password) {
    snackbar.error('两次输入的新密码不一致')
    return
  }
  passwordLoading.value = true
  try {
    await api.put('/profile/password', {
      old_password: passwordForm.value.old_password,
      new_password: passwordForm.value.new_password,
    })
    passwordForm.value = { old_password: '', new_password: '', confirm_password: '' }
    snackbar.success('密码修改成功')
  } catch (e) {
    snackbar.error(e.response?.data?.message || '密码修改失败')
  } finally {
    passwordLoading.value = false
  }
}
</script>

<template>
  <div>
    <div v-if="loading" class="d-flex justify-center align-center" style="min-height: 300px;">
      <VProgressCircular indeterminate color="primary" size="48" />
    </div>

    <VRow v-else>
      <!-- 左侧：头像 -->
      <VCol cols="12" md="4">
        <VCard data-no-hover>
          <VCardText class="d-flex flex-column align-center pa-8">
            <VAvatar size="120" class="mb-4">
              <VImg v-if="avatarPreview" :src="avatarPreview" cover />
              <VImg v-else :src="defaultAvatar" cover />
            </VAvatar>

            <div class="text-h6 font-weight-semibold mb-1">{{ profile.username }}</div>
            <div class="text-body-2 text-medium-emphasis mb-4">管理员</div>

            <VBtn
              variant="tonal"
              color="primary"
              size="small"
              class="mb-2"
              @click="$refs.avatarInput.click()"
            >
              <VIcon icon="ri-camera-fill" size="16" class="me-1" />
              选择头像
            </VBtn>
            <input
              ref="avatarInput"
              type="file"
              accept="image/*"
              style="display: none;"
              @change="onAvatarChange"
            />

            <VBtn
              v-if="avatarFile"
              color="primary"
              size="small"
              :loading="uploadLoading"
              @click="uploadAvatar"
            >
              上传头像
            </VBtn>

            <div class="text-caption text-medium-emphasis mt-2">
              支持 JPG、PNG 格式，不超过 2MB
            </div>
          </VCardText>
        </VCard>
      </VCol>

      <!-- 右侧：账号设置 -->
      <VCol cols="12" md="8">
        <!-- 修改用户名 -->
        <VCard class="mb-4" data-no-hover>
          <VCardTitle class="text-body-1 font-weight-semibold pa-4 pb-2">
            修改用户名
          </VCardTitle>
          <VCardText class="pa-4 pt-0">
            <VRow align="center">
              <VCol cols="12" sm="8">
                <VTextField
                  v-model="newUsername"
                  label="用户名"
                  density="compact"
                  variant="outlined"
                  hide-details
                />
              </VCol>
              <VCol cols="12" sm="4">
                <VBtn
                  color="primary"
                  block
                  :loading="usernameLoading"
                  @click="changeUsername"
                >
                  保存
                </VBtn>
              </VCol>
            </VRow>
          </VCardText>
        </VCard>

        <!-- 修改密码 -->
        <VCard data-no-hover>
          <VCardTitle class="text-body-1 font-weight-semibold pa-4 pb-2">
            修改密码
          </VCardTitle>
          <VCardText class="pa-4 pt-0">
            <VRow>
              <VCol cols="12">
                <VTextField
                  v-model="passwordForm.old_password"
                  label="原密码"
                  :type="showOldPwd ? 'text' : 'password'"
                  density="compact"
                  variant="outlined"
                  :append-inner-icon="showOldPwd ? 'ri-eye-off-line' : 'ri-eye-line'"
                  @click:append-inner="showOldPwd = !showOldPwd"
                />
              </VCol>
              <VCol cols="12">
                <VTextField
                  v-model="passwordForm.new_password"
                  label="新密码"
                  :type="showNewPwd ? 'text' : 'password'"
                  density="compact"
                  variant="outlined"
                  :append-inner-icon="showNewPwd ? 'ri-eye-off-line' : 'ri-eye-line'"
                  @click:append-inner="showNewPwd = !showNewPwd"
                />
              </VCol>
              <VCol cols="12">
                <VTextField
                  v-model="passwordForm.confirm_password"
                  label="确认新密码"
                  :type="showConfirmPwd ? 'text' : 'password'"
                  density="compact"
                  variant="outlined"
                  :append-inner-icon="showConfirmPwd ? 'ri-eye-off-line' : 'ri-eye-line'"
                  @click:append-inner="showConfirmPwd = !showConfirmPwd"
                />
              </VCol>
              <VCol cols="12">
                <VBtn
                  color="primary"
                  :loading="passwordLoading"
                  @click="changePassword"
                >
                  修改密码
                </VBtn>
              </VCol>
            </VRow>
          </VCardText>
        </VCard>
      </VCol>
    </VRow>
  </div>
</template>

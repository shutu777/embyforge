<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'
import avatar1 from '@images/avatars/avatar-1.png'

const router = useRouter()
const snackbar = useSnackbar()
const username = ref('管理员')
const avatarUrl = ref('')

// 获取用户信息
async function fetchProfile() {
  try {
    const res = await api.get('/profile')
    username.value = res.data.data.username || '管理员'
    avatarUrl.value = res.data.data.avatar || ''
  } catch (e) {
    // 静默失败
  }
}

onMounted(fetchProfile)

// 注销
function handleLogout() {
  localStorage.removeItem('token')
  snackbar.success('已成功注销')
  router.push({ name: 'login' })
}
</script>

<template>
  <VBadge
    dot
    location="bottom right"
    offset-x="3"
    offset-y="3"
    color="success"
    bordered
  >
    <VAvatar
      class="cursor-pointer"
      color="primary"
      variant="tonal"
    >
      <VImg v-if="avatarUrl" :src="avatarUrl" />
      <VImg v-else :src="avatar1" />

      <VMenu
        activator="parent"
        width="230"
        location="bottom end"
        offset="14px"
      >
        <VList>
          <!-- 用户信息 -->
          <VListItem>
            <template #prepend>
              <VListItemAction start>
                <VBadge
                  dot
                  location="bottom right"
                  offset-x="3"
                  offset-y="3"
                  color="success"
                >
                  <VAvatar
                    color="primary"
                    variant="tonal"
                  >
                    <VImg v-if="avatarUrl" :src="avatarUrl" />
                    <VImg v-else :src="avatar1" />
                  </VAvatar>
                </VBadge>
              </VListItemAction>
            </template>

            <VListItemTitle class="font-weight-semibold">
              {{ username }}
            </VListItemTitle>
            <VListItemSubtitle>管理员</VListItemSubtitle>
          </VListItem>
          <VDivider class="my-2" />

          <!-- 个人设置 -->
          <VListItem :to="{ name: 'profile' }" link>
            <template #prepend>
              <VIcon
                class="me-2"
                icon="ri-settings-4-line"
                size="22"
              />
            </template>
            <VListItemTitle>个人设置</VListItemTitle>
          </VListItem>

          <!-- 版本信息 -->
          <VListItem link href="https://github.com/shutu777/embyforge" target="_blank">
            <template #prepend>
              <VIcon
                class="me-2"
                icon="ri-github-fill"
                size="22"
              />
            </template>
            <VListItemTitle>v1.0.0</VListItemTitle>
          </VListItem>

          <VDivider class="my-2" />

          <!-- 注销 -->
          <VListItem link @click="handleLogout">
            <template #prepend>
              <VIcon
                class="me-2"
                icon="ri-logout-box-r-line"
                size="22"
              />
            </template>
            <VListItemTitle>注销</VListItemTitle>
          </VListItem>
        </VList>
      </VMenu>
    </VAvatar>
  </VBadge>
</template>

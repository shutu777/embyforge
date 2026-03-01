<script setup>
import { ref, onMounted } from 'vue'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'

const snackbar = useSnackbar()

// 表单数据
const cookie = ref('')
const cid = ref('')

// 状态
const saving = ref(false)
const testing = ref(false)
const pageLoading = ref(false)

// 页面加载时获取已保存配置
onMounted(async () => {
  pageLoading.value = true
  try {
    const { data } = await api.get('/system-config')
    if (data.data) {
      for (const config of data.data) {
        switch (config.key) {
          case 'pan115_cookie':
            cookie.value = config.value || ''
            break
          case 'pan115_cid':
            cid.value = config.value || ''
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

// 测试 Cookie
async function testCookie() {
  testing.value = true
  try {
    const { data } = await api.post('/pan115/test-cookie')
    const result = data.data
    if (result?.valid) {
      snackbar.success(result.message)
    } else {
      snackbar.error(result?.message || 'Cookie 无效')
    }
  } catch (e) {
    snackbar.error(e.response?.data?.message || '测试失败')
  } finally {
    testing.value = false
  }
}

// 保存配置
async function saveConfig() {
  if (!cookie.value.trim()) {
    snackbar.error('请输入 115 网盘 Cookie')
    return
  }
  saving.value = true
  try {
    await api.put('/system-config/pan115_cookie', { value: cookie.value.trim() })
    await api.put('/system-config/pan115_cid', { value: cid.value.trim() || '0' })
    snackbar.success('115 网盘配置保存成功')
  } catch (e) {
    snackbar.error(e.response?.data?.message || '保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div>
    <!-- 页面标题 -->
    <div class="mb-6">
      <h1 class="text-h4 font-weight-bold mb-2">115 网盘配置</h1>
      <p class="text-body-1 text-medium-emphasis">
        配置 115 网盘 Cookie 和存储目录，用于 HDHive 资源转存功能
      </p>
    </div>

    <!-- 配置表单 -->
    <VCard variant="flat" class="content-card" data-no-hover>
      <VCardText class="pa-6">
        <div class="d-flex align-center mb-6">
          <VAvatar color="info" variant="tonal" size="42" rounded="lg" class="me-3">
            <VIcon icon="ri-cloud-line" size="22" />
          </VAvatar>
          <div>
            <div class="text-body-1 font-weight-semibold">115 网盘设置</div>
            <div class="text-body-2 text-medium-emphasis">配置登录凭证和转存目标目录</div>
          </div>
        </div>

        <VRow>
          <VCol cols="12">
            <VTextarea
              v-model="cookie"
              label="115 Cookie"
              placeholder="从浏览器复制 115 网盘的 Cookie"
              hint="登录 115.com 后，从浏览器开发者工具的 Network 面板中复制 Cookie"
              persistent-hint
              rows="3"
              auto-grow
              :loading="pageLoading"
            />
          </VCol>
          <VCol cols="12" md="6">
            <VTextField
              v-model="cid"
              label="存储目录 ID (cid)"
              placeholder="填写目标文件夹的 cid，默认为根目录 (0)"
              hint="在 115 网盘中打开目标文件夹，URL 中的 cid 参数即为文件夹 ID"
              persistent-hint
              :loading="pageLoading"
            />
          </VCol>
        </VRow>

        <VDivider class="my-6" />

        <div class="d-flex justify-end ga-3">
          <VBtn
            variant="tonal"
            :loading="testing"
            prepend-icon="ri-shield-check-line"
            @click="testCookie"
          >
            测试 Cookie
          </VBtn>
          <VBtn
            color="primary"
            :loading="saving"
            prepend-icon="ri-save-line"
            @click="saveConfig"
          >
            保存配置
          </VBtn>
        </div>

        <!-- 使用说明 -->
        <VAlert
          type="info"
          variant="tonal"
          class="mt-6"
          density="compact"
        >
          <div class="text-body-2">
            <strong>如何获取 Cookie：</strong>登录
            <a href="https://115.com" target="_blank" class="text-info">115.com</a>
            → 打开浏览器开发者工具 (F12) → Network 面板 → 刷新页面 → 点击任意请求 → 复制 Request Headers 中的 Cookie 值
          </div>
          <div class="text-body-2 mt-2">
            <strong>如何获取 cid：</strong>在 115 网盘中打开你要保存文件的目标文件夹，浏览器地址栏 URL 中 <code>cid=</code> 后面的数字就是文件夹 ID
          </div>
        </VAlert>
      </VCardText>
    </VCard>
  </div>
</template>

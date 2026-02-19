<script setup>
import { ref, onMounted } from 'vue'
import api from '@/utils/api'
import { useSnackbar } from '@/composables/useSnackbar'

const snackbar = useSnackbar()

// 手动刷新表单状态
const manualForm = ref({
  symediaUrl: '',
  authToken: '',
})
const manualSaveLoading = ref(false)
const manualRefreshLoading = ref(false)

// GitHub 监听表单状态
const githubForm = ref({
  repoUrl: '',
  branch: 'main',
  filePath: '',
  secret: '',
})
const githubSaveLoading = ref(false)
const githubRefreshLoading = ref(false)
const webhookUrl = ref('')

// 页面加载状态
const pageLoading = ref(false)

// 密码可见性切换
const showAuthToken = ref(false)
const showSecret = ref(false)

// URL 格式验证规则
const urlRules = [
  v => !!v || 'URL 不能为空',
  v => /^https?:\/\/.+/.test(v) || 'URL 格式无效，必须以 http:// 或 https:// 开头',
]

// 必填字段验证规则
const requiredRules = [
  v => !!v || '此字段不能为空',
  v => (typeof v === 'string' && v.trim().length > 0) || '此字段不能为空',
]

// 处理API错误响应，返回友好的错误消息
function handleApiError(error, defaultMessage = '操作失败') {
  // 网络错误（无响应）
  if (!error.response) {
    if (error.code === 'ECONNABORTED' || error.message.includes('timeout')) {
      return '请求超时，请检查网络连接'
    }
    if (error.code === 'ERR_NETWORK' || error.message.includes('Network Error')) {
      return '网络连接失败，请检查网络设置'
    }
    return '网络错误，请稍后重试'
  }

  // 根据HTTP状态码返回友好消息
  const status = error.response.status
  const responseData = error.response.data

  switch (status) {
    case 400:
      // 请求参数错误
      return responseData?.error || responseData?.message || '请求参数错误，请检查输入'
    case 401:
      // 认证失败
      return '认证失败，请重新登录'
    case 403:
      // 权限不足
      return '权限不足，无法执行此操作'
    case 404:
      // 资源不存在
      return responseData?.error || responseData?.message || '请求的资源不存在'
    case 500:
      // 服务器内部错误
      return responseData?.error || responseData?.message || '服务器内部错误，请稍后重试'
    case 502:
      // 网关错误
      return '服务暂时不可用，请稍后重试'
    case 503:
      // 服务不可用
      return '服务维护中，请稍后重试'
    default:
      // 其他错误
      return responseData?.error || responseData?.message || defaultMessage
  }
}

// 加载已保存的配置
async function fetchConfigs() {
  pageLoading.value = true
  try {
    const { data } = await api.get('/symedia/config')
    if (data.data) {
      // 填充手动刷新表单
      if (data.data.symedia_url) {
        manualForm.value.symediaUrl = data.data.symedia_url
      }
      if (data.data.symedia_auth_token) {
        manualForm.value.authToken = data.data.symedia_auth_token
      }
      
      // 填充 GitHub 监听表单
      if (data.data.github_config) {
        const github = data.data.github_config
        githubForm.value.repoUrl = github.repo_url || ''
        githubForm.value.branch = github.branch || 'main'
        githubForm.value.filePath = github.file_path || ''
        githubForm.value.secret = github.secret || ''
        webhookUrl.value = github.webhook_url || ''
      }
    }
  } catch (e) {
    console.error('获取配置失败', e)
    // 不显示错误提示，因为可能是首次访问
  } finally {
    pageLoading.value = false
  }
}

// 保存手动刷新配置
async function handleSaveManualConfig() {
  // 验证表单
  if (!manualForm.value.symediaUrl || !manualForm.value.authToken) {
    snackbar.error('请填写所有必填字段')
    return
  }
  
  // 验证 URL 格式
  if (!/^https?:\/\/.+/.test(manualForm.value.symediaUrl)) {
    snackbar.error('Symedia 地址格式无效，必须以 http:// 或 https:// 开头')
    return
  }
  
  manualSaveLoading.value = true
  try {
    // 这里只保存配置，不触发刷新
    // 后端需要提供一个单独的保存配置接口
    await api.post('/symedia/save-config', {
      symedia_url: manualForm.value.symediaUrl,
      auth_token: manualForm.value.authToken,
    })
    snackbar.success('配置保存成功')
  } catch (e) {
    const errorMsg = handleApiError(e, '保存配置失败')
    snackbar.error(errorMsg)
    console.error('保存配置失败:', e)
  } finally {
    manualSaveLoading.value = false
  }
}

// 手动触发配置刷新
async function handleManualRefresh() {
  // 验证表单
  if (!manualForm.value.symediaUrl || !manualForm.value.authToken) {
    snackbar.error('请先保存配置')
    return
  }
  
  manualRefreshLoading.value = true
  try {
    const { data } = await api.post('/symedia/refresh', {
      symedia_url: manualForm.value.symediaUrl,
      auth_token: manualForm.value.authToken,
    })
    snackbar.success(data.message || '配置刷新成功')
  } catch (e) {
    const errorMsg = handleApiError(e, '配置刷新失败')
    snackbar.error(errorMsg)
    console.error('配置刷新失败:', e)
  } finally {
    manualRefreshLoading.value = false
  }
}

// 保存 GitHub 监听配置（不刷新 Webhook URL）
async function handleSaveGithubConfig() {
  // 验证表单（文件路径为可选）
  if (!githubForm.value.repoUrl || !githubForm.value.branch || !githubForm.value.secret) {
    snackbar.error('请填写所有必填字段')
    return
  }
  
  // 验证 URL 格式
  if (!/^https?:\/\/.+/.test(githubForm.value.repoUrl)) {
    snackbar.error('仓库 URL 格式无效，必须以 http:// 或 https:// 开头')
    return
  }
  
  githubSaveLoading.value = true
  try {
    await api.post('/symedia/github-config-save', {
      repo_url: githubForm.value.repoUrl,
      branch: githubForm.value.branch,
      file_path: githubForm.value.filePath || '*',
      secret: githubForm.value.secret,
    })
    snackbar.success('GitHub 配置保存成功')
  } catch (e) {
    const errorMsg = handleApiError(e, '保存配置失败')
    snackbar.error(errorMsg)
    console.error('保存GitHub配置失败:', e)
  } finally {
    githubSaveLoading.value = false
  }
}

// 刷新 Webhook URL
async function handleRefreshWebhookUrl() {
  // 验证表单
  if (!githubForm.value.repoUrl || !githubForm.value.branch || !githubForm.value.secret) {
    snackbar.error('请先保存配置')
    return
  }
  
  githubRefreshLoading.value = true
  try {
    const { data } = await api.post('/symedia/github-config', {
      repo_url: githubForm.value.repoUrl,
      branch: githubForm.value.branch,
      file_path: githubForm.value.filePath || '*',
      secret: githubForm.value.secret,
    })
    webhookUrl.value = data.data?.webhook_url || ''
    snackbar.success('Webhook URL 已刷新')
  } catch (e) {
    const errorMsg = handleApiError(e, '刷新 Webhook URL 失败')
    snackbar.error(errorMsg)
    console.error('刷新Webhook URL失败:', e)
  } finally {
    githubRefreshLoading.value = false
  }
}

// 复制 Webhook URL 到剪贴板
async function copyWebhookUrl() {
  if (!webhookUrl.value) {
    snackbar.error('Webhook URL 为空')
    return
  }
  
  try {
    await navigator.clipboard.writeText(webhookUrl.value)
    snackbar.success('Webhook URL 已复制到剪贴板')
  } catch (e) {
    snackbar.error('复制失败，请手动复制')
  }
}

// 页面加载时获取配置
onMounted(async () => {
  await fetchConfigs()
})
</script>

<template>
  <div class="symedia-config">
    <!-- 页面标题和说明 -->
    <div class="mb-6">
      <h1 class="text-h4 font-weight-bold mb-2">Symedia 配置刷新</h1>
      <p class="text-body-1 text-medium-emphasis">
        管理 Symedia 服务的配置更新，支持手动触发或通过 GitHub Webhook 自动监听仓库更新
      </p>
    </div>

    <!-- 加载状态 -->
    <div v-if="pageLoading" class="d-flex justify-center align-center" style="min-height: 300px;">
      <VProgressCircular indeterminate color="primary" size="48" />
    </div>

    <template v-else>
      <!-- 第一个卡片：手动配置刷新 -->
      <VCard class="dash-card mb-6" data-no-hover>
        <VCardText class="pa-6">
          <div class="d-flex align-center mb-5">
            <VAvatar color="primary" variant="tonal" size="42" rounded="lg" class="me-3">
              <VIcon icon="ri-refresh-line" size="22" />
            </VAvatar>
            <div>
              <div class="text-h6 font-weight-semibold">手动配置刷新</div>
              <div class="text-body-2 text-medium-emphasis">
                输入 Symedia 服务地址和认证令牌，立即触发配置更新
              </div>
            </div>
          </div>

          <VRow>
            <VCol cols="12" md="6">
              <VTextField
                v-model="manualForm.symediaUrl"
                label="Symedia 地址"
                placeholder="https://symedia.example.com:8096"
                :rules="urlRules"
                persistent-hint
                hint="完整的 Symedia 服务地址，包含协议和端口"
              >
                <template #prepend-inner>
                  <VIcon icon="ri-global-line" size="20" />
                </template>
              </VTextField>
            </VCol>

            <VCol cols="12" md="6">
              <VTextField
                v-model="manualForm.authToken"
                label="Authorization 令牌"
                placeholder="输入认证令牌"
                :type="showAuthToken ? 'text' : 'password'"
                :rules="requiredRules"
                persistent-hint
                hint="用于 Symedia API 认证的 JWT 令牌"
                :append-inner-icon="showAuthToken ? 'ri-eye-off-line' : 'ri-eye-line'"
                @click:append-inner="showAuthToken = !showAuthToken"
              >
                <template #prepend-inner>
                  <VIcon icon="ri-key-line" size="20" />
                </template>
              </VTextField>
            </VCol>

            <VCol cols="12">
              <div class="d-flex gap-2">
                <VBtn
                  color="primary"
                  variant="tonal"
                  :loading="manualSaveLoading"
                  :disabled="manualSaveLoading"
                  @click="handleSaveManualConfig"
                >
                  <VIcon icon="ri-save-line" class="me-1" />
                  保存配置
                </VBtn>
                <VBtn
                  color="primary"
                  :loading="manualRefreshLoading"
                  :disabled="manualRefreshLoading"
                  @click="handleManualRefresh"
                >
                  <VIcon icon="ri-loop-left-line" class="me-1" />
                  刷新配置
                </VBtn>
              </div>
            </VCol>
          </VRow>
        </VCardText>
      </VCard>

      <!-- 第二个卡片：GitHub 自动监听 -->
      <VCard class="dash-card" data-no-hover>
        <VCardText class="pa-6">
          <div class="d-flex align-center mb-5">
            <VAvatar color="success" variant="tonal" size="42" rounded="lg" class="me-3">
              <VIcon icon="ri-git-branch-line" size="22" />
            </VAvatar>
            <div>
              <div class="text-h6 font-weight-semibold">GitHub 自动监听</div>
              <div class="text-body-2 text-medium-emphasis">
                配置 GitHub Webhook，当规则文件变化时自动触发配置刷新
              </div>
            </div>
          </div>

          <!-- 配置说明 -->
          <VAlert type="info" variant="tonal" class="mb-5">
            <div class="text-body-2">
              <strong>配置步骤：</strong>
              <ol class="mt-2 mb-0 ps-4">
                <li>填写下方表单并保存配置</li>
                <li>复制生成的 Webhook URL</li>
                <li>在 GitHub 仓库设置中添加 Webhook</li>
                <li>将 Webhook URL 粘贴到 Payload URL 字段</li>
                <li>Content type 选择 application/json</li>
                <li>Secret 填写与下方相同的密钥</li>
                <li>选择 "Just the push event" 触发事件</li>
              </ol>
            </div>
          </VAlert>

          <VRow>
            <VCol cols="12" md="6">
              <VTextField
                v-model="githubForm.repoUrl"
                label="仓库 URL"
                placeholder="https://github.com/username/repo"
                :rules="urlRules"
                persistent-hint
                hint="GitHub 仓库的完整 URL"
              >
                <template #prepend-inner>
                  <VIcon icon="ri-github-fill" size="20" />
                </template>
              </VTextField>
            </VCol>

            <VCol cols="12" md="6">
              <VTextField
                v-model="githubForm.branch"
                label="分支名称"
                placeholder="main"
                :rules="requiredRules"
                persistent-hint
                hint="要监听的分支名称"
              >
                <template #prepend-inner>
                  <VIcon icon="ri-git-branch-line" size="20" />
                </template>
              </VTextField>
            </VCol>

            <VCol cols="12" md="6">
              <VTextField
                v-model="githubForm.filePath"
                label="监听的文件路径（可选）"
                placeholder="留空或输入 * 监听所有文件"
                persistent-hint
                hint="指定文件路径（如 config/rules.json）或留空监听所有文件变化"
              >
                <template #prepend-inner>
                  <VIcon icon="ri-file-line" size="20" />
                </template>
              </VTextField>
            </VCol>

            <VCol cols="12" md="6">
              <VTextField
                v-model="githubForm.secret"
                label="Webhook 密钥"
                placeholder="输入密钥"
                :type="showSecret ? 'text' : 'password'"
                :rules="requiredRules"
                persistent-hint
                hint="用于验证 GitHub Webhook 请求的密钥"
                :append-inner-icon="showSecret ? 'ri-eye-off-line' : 'ri-eye-line'"
                @click:append-inner="showSecret = !showSecret"
              >
                <template #prepend-inner>
                  <VIcon icon="ri-lock-password-line" size="20" />
                </template>
              </VTextField>
            </VCol>

            <VCol cols="12">
              <div class="d-flex gap-2">
                <VBtn
                  color="success"
                  :loading="githubSaveLoading"
                  :disabled="githubSaveLoading"
                  @click="handleSaveGithubConfig"
                >
                  <VIcon icon="ri-save-line" class="me-1" />
                  保存配置
                </VBtn>
                <VBtn
                  color="success"
                  variant="tonal"
                  :loading="githubRefreshLoading"
                  :disabled="githubRefreshLoading"
                  @click="handleRefreshWebhookUrl"
                >
                  <VIcon icon="ri-refresh-line" class="me-1" />
                  刷新 Webhook URL
                </VBtn>
              </div>
            </VCol>

            <!-- 生成的 Webhook URL -->
            <VCol v-if="webhookUrl" cols="12">
              <VCard variant="tonal" color="success">
                <VCardText class="pa-4">
                  <div class="text-body-2 font-weight-semibold mb-2">
                    <VIcon icon="ri-check-line" size="18" class="me-1" />
                    Webhook URL 已生成
                  </div>
                  <div class="d-flex align-center">
                    <VTextField
                      :model-value="webhookUrl"
                      readonly
                      density="compact"
                      variant="outlined"
                      hide-details
                      class="me-2"
                    />
                    <VBtn
                      color="success"
                      variant="tonal"
                      @click="copyWebhookUrl"
                    >
                      <VIcon icon="ri-file-copy-line" class="me-1" />
                      复制
                    </VBtn>
                  </div>
                </VCardText>
              </VCard>
            </VCol>
          </VRow>
        </VCardText>
      </VCard>
    </template>
  </div>
</template>

<style lang="scss" scoped>
.dash-card {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  transition: transform 0.2s ease, box-shadow 0.2s ease;

  &:hover {
    transform: translateY(-2px);
    box-shadow: 0 4px 20px rgba(0, 0, 0, 0.1);
  }
}
</style>

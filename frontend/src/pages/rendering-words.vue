<script setup>
import { ref, computed, watch, onMounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { useSnackbar } from '@/composables/useSnackbar'
import { v4 as uuidv4 } from 'uuid'
import api from '@/utils/api'

const { t } = useI18n()
const snackbar = useSnackbar()

// LocalStorage 键名
const STORAGE_KEY = 'rendering_words_series_list'

// 从 localStorage 加载数据
function loadFromStorage() {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored) {
      const parsed = JSON.parse(stored)
      if (Array.isArray(parsed)) {
        seriesList.value = parsed
      }
    }
  } catch (error) {
    console.error('加载数据失败:', error)
  }
}

// 保存到 localStorage
function saveToStorage() {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(seriesList.value))
  } catch (error) {
    console.error('保存数据失败:', error)
  }
}

// 当前编辑的剧集
const currentSeries = ref({
  id: null,
  name: '',
  tmdbId: '',
  type: 'tv',
  rules: []
})

// 剧集列表
const seriesList = ref([])

// 选中的剧集
const selectedSeries = ref([])

// 导入对话框
const importDialog = ref(false)
const importLoading = ref(false)
const importCandidates = ref([])
const importSearch = ref('')
const importPage = ref(1)
const importPageSize = ref(20)
const importTotal = ref(0)

// 表单验证
const formValid = ref(false)
const formRef = ref(null)
const nameRules = [
  v => !!v || t('renderingWords.validation.seriesNameRequired'),
]
const tmdbIdRules = [
  v => !!v || t('renderingWords.validation.tmdbIdRequired'),
  v => {
    const str = String(v)
    return /^\d+$/.test(str) || t('renderingWords.validation.tmdbIdInvalid')
  },
]

// 添加新规则
function addRule() {
  currentSeries.value.rules.push({
    id: uuidv4(),
    sourceSeason: '',
    sourceEpisodes: '',
    targetSeason: '',
    offset: ''
  })
}

// 删除规则
function deleteRule(ruleId) {
  const index = currentSeries.value.rules.findIndex(r => r.id === ruleId)
  if (index > -1) {
    currentSeries.value.rules.splice(index, 1)
  }
}

// 清空所有规则
function clearAllRules() {
  if (currentSeries.value.rules.length === 0) return
  
  if (confirm(t('renderingWords.messages.confirmClearAll'))) {
    currentSeries.value.rules = []
  }
}

// 生成单条规则的文本
function generateRuleText(rule, tmdbId, type) {
  return `@?{[tmdbid=${tmdbId};type=${type};s=${rule.sourceSeason};e=${rule.sourceEpisodes}]} => {[s=${rule.targetSeason};e=${rule.offset}]}`
}

// 生成剧集的完整配置文本
function generateSeriesText(series) {
  if (!series.name || !series.tmdbId || series.rules.length === 0) {
    return ''
  }
  
  let text = `# ${series.name}\n`
  series.rules.forEach(rule => {
    text += generateRuleText(rule, series.tmdbId, series.type) + '\n'
  })
  return text
}

// 预览文本 - 显示选中的剧集
const previewText = computed(() => {
  if (selectedSeries.value.length === 0) {
    // 如果没有选中剧集，显示当前编辑的剧集
    return generateSeriesText(currentSeries.value)
  }
  
  // 显示所有选中的剧集
  let text = ''
  selectedSeries.value.forEach(id => {
    const series = seriesList.value.find(s => s.id === id)
    if (series) {
      text += generateSeriesText(series) + '\n'
    }
  })
  return text.trim()
})

// 复制单条规则
async function copyRule(rule) {
  if (!currentSeries.value.tmdbId) {
    snackbar.error(t('renderingWords.validation.tmdbIdRequired'))
    return
  }
  
  const text = generateRuleText(rule, currentSeries.value.tmdbId, currentSeries.value.type)
  try {
    await navigator.clipboard.writeText(text)
    snackbar.success(t('renderingWords.messages.copiedSuccess'))
  } catch (err) {
    snackbar.error(t('renderingWords.messages.copyFailed'))
  }
}

// 复制全部
async function copyAll() {
  const text = previewText.value
  if (!text) {
    if (selectedSeries.value.length === 0) {
      snackbar.error(t('renderingWords.validation.atLeastOneRule'))
    } else {
      snackbar.error('请选择要复制的剧集')
    }
    return
  }
  
  try {
    await navigator.clipboard.writeText(text)
    if (selectedSeries.value.length > 0) {
      snackbar.success(t('renderingWords.messages.batchCopied', { count: selectedSeries.value.length }))
    } else {
      snackbar.success(t('renderingWords.messages.copiedSuccess'))
    }
  } catch (err) {
    snackbar.error(t('renderingWords.messages.copyFailed'))
  }
}

// 保存剧集
async function saveSeries() {
  // 先触发表单验证
  if (formRef.value) {
    const { valid } = await formRef.value.validate()
    if (!valid) {
      return
    }
  }
  
  if (!currentSeries.value.name || !currentSeries.value.tmdbId || currentSeries.value.rules.length === 0) {
    snackbar.error(t('renderingWords.validation.atLeastOneRule'))
    return
  }
  
  // 验证所有规则
  for (const rule of currentSeries.value.rules) {
    if (!rule.sourceSeason || !rule.sourceEpisodes || !rule.targetSeason || !rule.offset) {
      snackbar.error('请填写完整的规则信息')
      return
    }
  }
  
  if (currentSeries.value.id) {
    // 更新现有剧集
    const index = seriesList.value.findIndex(s => s.id === currentSeries.value.id)
    if (index > -1) {
      seriesList.value[index] = JSON.parse(JSON.stringify(currentSeries.value))
      snackbar.success(t('renderingWords.messages.seriesUpdated'))
    }
  } else {
    // 添加新剧集
    const newSeries = {
      ...JSON.parse(JSON.stringify(currentSeries.value)),
      id: uuidv4()
    }
    seriesList.value.push(newSeries)
    snackbar.success(t('renderingWords.messages.seriesAdded'))
  }
  
  // 保存到 localStorage
  saveToStorage()
  
  // 重置表单
  resetForm()
}

// 重置表单
async function resetForm() {
  currentSeries.value = {
    id: null,
    name: '',
    tmdbId: '',
    type: 'tv',
    rules: []
  }
  
  // 等待数据更新到 DOM
  await nextTick()
  
  // 重置表单验证状态
  if (formRef.value) {
    formRef.value.resetValidation()
  }
}

// 编辑剧集
async function editSeries(series) {
  currentSeries.value = JSON.parse(JSON.stringify(series))
  
  // 等待数据绑定到表单
  await nextTick()
  
  // 重置表单验证状态
  if (formRef.value) {
    formRef.value.resetValidation()
  }
}

// 删除剧集
function deleteSeries(seriesId) {
  if (confirm(t('renderingWords.messages.confirmDelete'))) {
    const index = seriesList.value.findIndex(s => s.id === seriesId)
    if (index > -1) {
      seriesList.value.splice(index, 1)
      saveToStorage()
      snackbar.success(t('renderingWords.messages.seriesDeleted'))
    }
  }
}

// 复制剧集配置
async function copySeriesConfig(series) {
  const text = generateSeriesText(series)
  try {
    await navigator.clipboard.writeText(text)
    snackbar.success(t('renderingWords.messages.copiedSuccess'))
  } catch (err) {
    snackbar.error(t('renderingWords.messages.copyFailed'))
  }
}

// 批量删除
function batchDelete() {
  if (selectedSeries.value.length === 0) {
    snackbar.error('请选择要删除的剧集')
    return
  }
  
  if (confirm(t('renderingWords.messages.confirmBatchDelete', { count: selectedSeries.value.length }))) {
    seriesList.value = seriesList.value.filter(s => !selectedSeries.value.includes(s.id))
    selectedSeries.value = []
    saveToStorage()
    snackbar.success(t('renderingWords.messages.batchDeleted', { count: selectedSeries.value.length }))
  }
}

// 导出文件
function exportFile() {
  let text = ''
  seriesList.value.forEach(series => {
    text += generateSeriesText(series) + '\n'
  })
  
  const blob = new Blob([text], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'custom_rendering_words.txt'
  a.click()
  URL.revokeObjectURL(url)
  snackbar.success(t('renderingWords.messages.exportSuccess'))
}

// 全选/取消全选
const allSelected = computed({
  get: () => selectedSeries.value.length === seriesList.value.length && seriesList.value.length > 0,
  set: (val) => {
    if (val) {
      selectedSeries.value = seriesList.value.map(s => s.id)
    } else {
      selectedSeries.value = []
    }
  }
})

// 获取导入候选列表
async function fetchImportCandidates() {
  importLoading.value = true
  try {
    const { data } = await api.get('/rendering-words/import-candidates', {
      params: {
        search: importSearch.value,
        page: importPage.value,
        pageSize: importPageSize.value
      }
    })
    importCandidates.value = data.data || []
    importTotal.value = data.total || 0
  } catch (error) {
    console.error('获取导入候选失败:', error)
    snackbar.error('获取导入候选失败')
  } finally {
    importLoading.value = false
  }
}

// 打开导入对话框
function openImportDialog() {
  importDialog.value = true
  importSearch.value = ''
  importPage.value = 1
  fetchImportCandidates()
}

// 导入剧集
async function importSeries(candidate) {
  // 先关闭对话框
  importDialog.value = false
  
  // 等待对话框关闭动画完成
  await nextTick()
  
  // 设置数据
  currentSeries.value = {
    id: null,
    name: candidate.name,
    tmdbId: String(candidate.tmdb_id),
    type: 'tv',
    rules: candidate.recommended_rules.map(rule => ({
      id: uuidv4(),
      sourceSeason: String(rule.source_season),
      sourceEpisodes: rule.source_episodes,
      targetSeason: String(rule.target_season),
      offset: rule.offset
    }))
  }
  
  // 再等待一次，确保数据已经绑定到表单
  await nextTick()
  
  // 重置表单验证状态
  if (formRef.value) {
    formRef.value.resetValidation()
  }
  
  snackbar.success(t('renderingWords.messages.importSuccess'))
}

// 监听搜索变化
watch(importSearch, () => {
  importPage.value = 1
  fetchImportCandidates()
})

// 监听页码变化
watch(importPage, () => {
  fetchImportCandidates()
})

// 页面加载时从 localStorage 加载数据
onMounted(() => {
  loadFromStorage()
})


</script>

<template>
  <div class="rendering-words-page">
    <!-- 页面标题 -->
    <div class="page-header mb-6">
      <div class="d-flex align-center gap-3 mb-2">
        <VIcon
          icon="ri-code-s-slash-line"
          size="32"
          color="primary"
        />
        <h2 class="text-h4 font-weight-bold">
          {{ t('renderingWords.title') }}
        </h2>
      </div>
      <div class="d-flex justify-space-between align-center">
        <p class="text-body-1 text-medium-emphasis mb-0">
          {{ t('renderingWords.description') }}
        </p>
        <VBtn
          href="https://wiki.viplee.cc/symedia/advanced/custom_rendering_words/"
          target="_blank"
          variant="tonal"
          color="primary"
          prepend-icon="ri-book-open-line"
        >
          {{ t('renderingWords.wikiLink') }}
        </VBtn>
      </div>
    </div>

    <!-- 第一行：编辑剧集 + 剧集列表 -->
    <VRow class="equal-height-row">
      <!-- 左侧: 编辑剧集 -->
      <VCol
        cols="12"
        md="6"
        class="d-flex"
      >
        <VCard class="flex-grow-1" data-no-hover>
          <VCardTitle class="pa-4">
            <div class="d-flex justify-space-between align-center">
              <div class="d-flex align-center gap-2">
                <VIcon
                  icon="ri-edit-box-line"
                  color="primary"
                />
                <span>编辑剧集</span>
              </div>
              <VBtn
                color="primary"
                prepend-icon="ri-download-line"
                size="small"
                @click="openImportDialog"
              >
                {{ t('renderingWords.importFromMapping') }}
              </VBtn>
            </div>
          </VCardTitle>
          <VDivider />
          <VCardText class="pa-4">
            <VForm
              ref="formRef"
              v-model="formValid"
              @submit.prevent="saveSeries"
            >
              <!-- 基本信息 -->
              <VTextField
                v-model="currentSeries.name"
                :label="t('renderingWords.seriesName')"
                :placeholder="t('renderingWords.seriesNamePlaceholder')"
                :rules="nameRules"
                variant="outlined"
                density="comfortable"
                class="mb-3"
              />

              <VRow>
                <VCol
                  cols="8"
                >
                  <VTextField
                    v-model="currentSeries.tmdbId"
                    :label="t('renderingWords.tmdbId')"
                    :placeholder="t('renderingWords.tmdbIdPlaceholder')"
                    :rules="tmdbIdRules"
                    variant="outlined"
                    density="comfortable"
                    type="number"
                  />
                </VCol>
                <VCol
                  cols="4"
                >
                  <VSelect
                    v-model="currentSeries.type"
                    :label="t('renderingWords.type')"
                    :items="[
                      { title: t('renderingWords.typeTv'), value: 'tv' },
                      { title: t('renderingWords.typeMovie'), value: 'movie' }
                    ]"
                    variant="outlined"
                    density="comfortable"
                  />
                </VCol>
              </VRow>

              <VDivider class="my-4" />

              <!-- 映射规则 -->
              <div class="d-flex justify-space-between align-center mb-3">
                <span class="text-subtitle-2">{{ t('renderingWords.mappingRules') }}</span>
                <div class="d-flex gap-2">
                  <VBtn
                    v-if="currentSeries.rules.length > 0"
                    size="small"
                    variant="text"
                    color="error"
                    @click="clearAllRules"
                  >
                    {{ t('renderingWords.clearAll') }}
                  </VBtn>
                  <VBtn
                    size="small"
                    color="primary"
                    prepend-icon="ri-add-line"
                    @click="addRule"
                  >
                    {{ t('renderingWords.addRule') }}
                  </VBtn>
                </div>
              </div>

              <!-- 规则列表 -->
              <div
                v-if="currentSeries.rules.length === 0"
                class="text-center py-6 text-medium-emphasis"
              >
                <VIcon
                  icon="ri-file-list-3-line"
                  size="40"
                  class="mb-2"
                />
                <p class="text-caption">暂无规则</p>
              </div>

              <TransitionGroup name="list">
                <VCard
                  v-for="(rule, index) in currentSeries.rules"
                  :key="rule.id"
                  variant="outlined"
                  class="mb-2 rule-card"
                >
                  <VCardText class="pa-3">
                    <div class="d-flex justify-space-between align-center mb-2">
                      <VChip
                        size="small"
                        color="primary"
                        variant="tonal"
                      >
                        {{ index + 1 }}
                      </VChip>
                      <VBtn
                        icon="ri-delete-bin-line"
                        size="x-small"
                        variant="text"
                        color="error"
                        @click="deleteRule(rule.id)"
                      />
                    </div>

                    <VRow dense>
                      <VCol cols="6" sm="3">
                        <VTextField
                          v-model="rule.sourceSeason"
                          :label="t('renderingWords.sourceSeason')"
                          variant="outlined"
                          density="compact"
                          type="number"
                          hide-details
                        />
                      </VCol>
                      <VCol cols="6" sm="3">
                        <VTextField
                          v-model="rule.sourceEpisodes"
                          :label="t('renderingWords.sourceEpisodes')"
                          :placeholder="t('renderingWords.sourceEpisodesPlaceholder')"
                          variant="outlined"
                          density="compact"
                          hide-details
                        />
                      </VCol>
                      <VCol cols="6" sm="3">
                        <VTextField
                          v-model="rule.targetSeason"
                          :label="t('renderingWords.targetSeason')"
                          variant="outlined"
                          density="compact"
                          type="number"
                          hide-details
                        />
                      </VCol>
                      <VCol cols="6" sm="3">
                        <VTextField
                          v-model="rule.offset"
                          :label="t('renderingWords.offset')"
                          :placeholder="t('renderingWords.offsetPlaceholder')"
                          variant="outlined"
                          density="compact"
                          hide-details
                        />
                      </VCol>
                    </VRow>

                    <div class="mt-2">
                      <VBtn
                        size="x-small"
                        variant="text"
                        prepend-icon="ri-file-copy-line"
                        @click="copyRule(rule)"
                      >
                        {{ t('renderingWords.copyLine') }}
                      </VBtn>
                    </div>
                  </VCardText>
                </VCard>
              </TransitionGroup>

              <!-- 操作按钮 -->
              <VRow dense class="mt-3">
                <VCol cols="6">
                  <VBtn
                    type="submit"
                    color="primary"
                    block
                    size="large"
                  >
                    {{ currentSeries.id ? '更新' : '保存' }}
                  </VBtn>
                </VCol>
                <VCol cols="6">
                  <VBtn
                    color="secondary"
                    block
                    size="large"
                    @click="resetForm"
                  >
                    重置
                  </VBtn>
                </VCol>
              </VRow>
            </VForm>
          </VCardText>
        </VCard>
      </VCol>

      <!-- 右侧: 剧集列表 -->
      <VCol
        cols="12"
        md="6"
        class="d-flex"
      >
        <VCard class="flex-grow-1" data-no-hover>
          <VCardTitle class="pa-4">
            <div class="d-flex justify-space-between align-center">
              <div class="d-flex align-center gap-2">
                <VIcon
                  icon="ri-tv-line"
                  color="primary"
                />
                <span>{{ t('renderingWords.seriesList') }}</span>
                <VChip
                  v-if="seriesList.length > 0"
                  size="small"
                  color="primary"
                  variant="tonal"
                >
                  {{ seriesList.length }}
                </VChip>
              </div>
              <VBtn
                v-if="seriesList.length > 0"
                size="small"
                color="success"
                prepend-icon="ri-download-line"
                @click="exportFile"
              >
                {{ t('renderingWords.exportFile') }}
              </VBtn>
            </div>
          </VCardTitle>
          <VDivider />
          <VCardText class="pa-4">
            <!-- 批量操作 -->
            <div
              v-if="seriesList.length > 0"
              class="d-flex justify-space-between align-center pa-3 mb-3 rounded batch-actions-bar"
            >
              <div class="d-flex align-center gap-2">
                <VCheckbox
                  v-model="allSelected"
                  hide-details
                  density="compact"
                />
                <span class="text-body-2">
                  {{ selectedSeries.length > 0 ? `已选 ${selectedSeries.length}` : '全选' }}
                </span>
              </div>
              <div v-if="selectedSeries.length > 0" class="d-flex gap-2">
                <VBtn
                  size="small"
                  color="error"
                  prepend-icon="ri-delete-bin-line"
                  @click="batchDelete"
                >
                  删除
                </VBtn>
              </div>
            </div>

            <!-- 列表 -->
            <div
              v-if="seriesList.length === 0"
              class="empty-series-container"
            >
              <VIcon
                icon="ri-tv-line"
                size="40"
              />
              <p class="text-caption mb-0">{{ t('renderingWords.noSeries') }}</p>
            </div>

            <TransitionGroup name="list">
              <VCard
                v-for="series in seriesList"
                :key="series.id"
                variant="outlined"
                class="mb-2 series-item"
              >
                <VCardText class="d-flex align-center pa-3">
                  <VCheckbox
                    v-model="selectedSeries"
                    :value="series.id"
                    hide-details
                    density="compact"
                    class="me-2"
                  />
                  <div class="flex-grow-1">
                    <div class="text-body-2 mb-1">{{ series.name }}</div>
                    <div class="d-flex gap-1">
                      <VChip size="x-small" variant="tonal">
                        {{ series.tmdbId }}
                      </VChip>
                      <VChip size="x-small" color="secondary" variant="tonal">
                        {{ series.rules.length }} 规则
                      </VChip>
                    </div>
                  </div>
                  <div class="d-flex gap-1">
                    <VBtn
                      icon="ri-file-copy-line"
                      size="x-small"
                      variant="text"
                      @click="copySeriesConfig(series)"
                    />
                    <VBtn
                      icon="ri-edit-line"
                      size="x-small"
                      variant="text"
                      @click="editSeries(series)"
                    />
                    <VBtn
                      icon="ri-delete-bin-line"
                      size="x-small"
                      variant="text"
                      color="error"
                      @click="deleteSeries(series.id)"
                    />
                  </div>
                </VCardText>
              </VCard>
            </TransitionGroup>
          </VCardText>
        </VCard>
      </VCol>
    </VRow>

    <!-- 第二行：预览 -->
    <VRow class="mt-3">
      <VCol cols="12">
        <VCard data-no-hover>
          <VCardTitle class="pa-4">
            <div class="d-flex justify-space-between align-center">
              <div class="d-flex align-center gap-2">
                <VIcon
                  icon="ri-eye-line"
                  color="primary"
                />
                <span>{{ t('renderingWords.preview') }}</span>
              </div>
              <VBtn
                size="small"
                color="primary"
                prepend-icon="ri-file-copy-line"
                @click="copyAll"
                :disabled="!previewText"
              >
                {{ t('renderingWords.copyAll') }}
              </VBtn>
            </div>
          </VCardTitle>
          <VDivider />
          <VCardText class="pa-0">
            <div class="preview-container">
              <pre v-if="previewText" class="preview-text">{{ previewText }}</pre>
              <div v-else class="preview-empty">
                <VIcon icon="ri-code-s-slash-line" size="40" class="mb-2" />
                <p class="text-caption text-medium-emphasis">填写表单后将在此处显示预览</p>
              </div>
            </div>
          </VCardText>
        </VCard>
      </VCol>
    </VRow>

    <!-- 导入对话框 -->
    <VDialog
      v-model="importDialog"
      max-width="800"
      scrollable
      data-no-hover
    >
      <VCard>
        <VCardTitle class="d-flex justify-space-between align-center pa-4">
          <div class="d-flex align-center gap-2">
            <VIcon
              icon="ri-download-cloud-line"
              color="primary"
            />
            <span class="font-weight-bold">{{ t('renderingWords.importDialog.title') }}</span>
          </div>
          <VBtn
            icon="ri-close-line"
            variant="text"
            size="small"
            @click="importDialog = false"
          />
        </VCardTitle>
        <VDivider />
        <VCardText class="pa-4">
          <p class="text-body-2 text-medium-emphasis mb-4">
            {{ t('renderingWords.importDialog.description') }}
          </p>

          <!-- 搜索框 -->
          <VTextField
            v-model="importSearch"
            :label="t('renderingWords.importDialog.search')"
            prepend-inner-icon="ri-search-line"
            variant="outlined"
            density="comfortable"
            clearable
            class="mb-4"
          />

          <!-- 加载状态 -->
          <div
            v-if="importLoading"
            class="text-center py-8"
          >
            <VProgressCircular
              indeterminate
              color="primary"
            />
            <p class="mt-2 text-medium-emphasis">
              {{ t('renderingWords.importDialog.loading') }}
            </p>
          </div>

          <!-- 候选列表 -->
          <div v-else-if="importCandidates.length > 0">
            <TransitionGroup name="list">
              <VCard
                v-for="candidate in importCandidates"
                :key="candidate.emby_item_id"
                variant="outlined"
                class="mb-3 import-item"
                elevation="0"
              >
                <VCardText class="pa-3">
                  <div class="d-flex justify-space-between align-center mb-2">
                    <div>
                      <div class="font-weight-medium text-body-2 mb-1">
                        {{ candidate.name }}
                      </div>
                      <VChip
                        size="x-small"
                        color="primary"
                        variant="tonal"
                      >
                        TMDB: {{ candidate.tmdb_id }}
                      </VChip>
                    </div>
                    <VBtn
                      size="small"
                      color="primary"
                      variant="tonal"
                      prepend-icon="ri-download-line"
                      @click="importSeries(candidate)"
                    >
                      导入
                    </VBtn>
                  </div>

                  <!-- 推荐规则 -->
                  <div
                    v-if="candidate.recommended_rules && candidate.recommended_rules.length > 0"
                    class="mt-2"
                  >
                    <div class="text-caption text-medium-emphasis mb-1 font-weight-medium">
                      {{ t('renderingWords.importDialog.recommended') }}:
                    </div>
                    <div class="recommended-rules pa-2 rounded">
                      <div
                        v-for="(rule, index) in candidate.recommended_rules"
                        :key="index"
                        class="text-caption"
                        style="font-family: 'Courier New', monospace;"
                      >
                        s={{ rule.source_season }}, e={{ rule.source_episodes }} => s={{ rule.target_season }}, e={{ rule.offset }}
                      </div>
                    </div>
                  </div>
                </VCardText>
              </VCard>
            </TransitionGroup>

            <!-- 分页 -->
            <div class="d-flex justify-center mt-4">
              <VPagination
                v-model="importPage"
                :length="Math.ceil(importTotal / importPageSize)"
                :total-visible="5"
              />
            </div>
          </div>

          <!-- 无数据 -->
          <div
            v-else
            class="empty-state text-center py-8"
          >
            <VIcon
              icon="ri-inbox-line"
              size="48"
              color="grey-lighten-1"
              class="mb-2"
            />
            <p class="text-body-2 text-medium-emphasis">{{ t('renderingWords.importDialog.noData') }}</p>
          </div>
        </VCardText>
        <VDivider />
        <VCardActions>
          <VSpacer />
          <VBtn
            variant="text"
            @click="importDialog = false"
          >
            {{ t('renderingWords.importDialog.cancel') }}
          </VBtn>
        </VCardActions>
      </VCard>
    </VDialog>
  </div>
</template>

<style scoped>
/* 强制禁用三个主卡片的 hover 效果 - 使用最高优先级 */
:deep(.v-card[data-no-hover]) {
  transition: none !important;
  transform: none !important;
  will-change: auto !important;
  cursor: default !important;
}

:deep(.v-card[data-no-hover]:hover) {
  transition: none !important;
  transform: none !important;
  box-shadow: 0px 2px 1px -1px var(--v-shadow-key-umbra-opacity, rgba(0, 0, 0, 0.2)), 0px 1px 1px 0px var(--v-shadow-key-penumbra-opacity, rgba(0, 0, 0, 0.14)), 0px 1px 3px 0px var(--v-shadow-key-ambient-opacity, rgba(0, 0, 0, 0.12)) !important;
  z-index: auto !important;
}

:deep(.v-card[data-no-hover]::before) {
  display: none !important;
}

/* 列表动画 */
.list-enter-active,
.list-leave-active {
  transition: all 0.3s ease;
}

.list-enter-from {
  opacity: 0;
  transform: translateY(-10px);
}

.list-leave-to {
  opacity: 0;
  transform: translateX(10px);
}

/* 规则卡片 */
.rule-card {
  /* 移除 hover 效果 */
}

/* 剧集列表项 */
.series-item {
  /* 移除 hover 效果 */
}

/* 导入项 */
.import-item {
  /* 移除 hover 效果 */
}

/* 批量操作栏 - 使用 primary 颜色 */
.batch-actions-bar {
  background-color: rgba(var(--v-theme-primary), 0.08);
  border: 1px solid rgba(var(--v-theme-primary), 0.2);
}

/* 预览容器 */
.preview-container {
  min-height: 200px;
  max-height: 400px;
  overflow: auto;
  padding: 16px;
  background-color: rgb(var(--v-theme-surface));
  border-top: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}

.preview-text {
  margin: 0;
  font-family: 'Courier New', 'Consolas', 'Monaco', monospace;
  font-size: 13px;
  line-height: 1.6;
  color: rgb(var(--v-theme-on-surface));
  white-space: pre-wrap;
  word-wrap: break-word;
}

.preview-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 200px;
  text-align: center;
}

/* 推荐规则 */
.recommended-rules {
  background-color: rgba(var(--v-theme-primary), 0.08);
  border: 1px solid rgba(var(--v-theme-primary), 0.2);
  border-left: 3px solid rgb(var(--v-theme-primary));
}

.recommended-rules .text-caption {
  color: rgb(var(--v-theme-on-surface));
  opacity: 0.9;
}

/* 暂无剧集的空状态 */
.empty-series-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 300px;
  color: rgba(var(--v-theme-on-surface), 0.6);
  gap: 8px;
}

/* 滚动条 */
.preview-container::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}

.preview-container::-webkit-scrollbar-track {
  background: transparent;
}

.preview-container::-webkit-scrollbar-thumb {
  background: rgba(var(--v-theme-on-surface), 0.2);
  border-radius: 4px;
}

/* 等高容器 */
.equal-height-row {
  display: flex;
  flex-wrap: wrap;
}

.equal-height-row > .v-col {
  display: flex;
}

.equal-height-row .v-card {
  width: 100%;
  display: flex;
  flex-direction: column;
}
</style>

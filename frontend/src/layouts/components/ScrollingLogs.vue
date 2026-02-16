<script setup>
import { ref, onBeforeUnmount, computed } from 'vue'
import api from '@/utils/api'

const dialog = ref(false)
const logs = ref([])
const loading = ref(false)
const activeFilter = ref('ALL')
let timer = null
const logContainer = ref(null)

const filters = ['ALL', 'INFO', 'WARNING', 'ERROR']

// 获取日志
async function fetchLogs() {
  if (loading.value) return
  loading.value = true
  try {
    const res = await api.get('/logs/recent')
    logs.value = res.data.data || []
  } catch (e) {
    // 静默失败
  } finally {
    loading.value = false
  }
}

function openDialog() {
  dialog.value = true
  fetchLogs()
  timer = setInterval(fetchLogs, 3000)
}

function closeDialog() {
  dialog.value = false
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

const filteredLogs = computed(() => {
  const source = activeFilter.value === 'ALL'
    ? logs.value
    : logs.value.filter(l => l.level === activeFilter.value)
  // 倒序：最新的在最上面
  return [...source].reverse()
})

function levelColor(level) {
  if (level === 'ERROR') return 'error'
  if (level === 'WARNING') return 'warning'
  return 'info'
}

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <!-- 触发按钮 -->
  <IconBtn @click="openDialog">
    <VIcon icon="ri-terminal-box-fill" size="22" />
  </IconBtn>

  <!-- 日志弹框 -->
  <VDialog
    v-model="dialog"
    persistent
    no-click-animation
    content-class="log-dialog-overlay"
    @update:model-value="val => { if (!val) closeDialog() }"
  >
    <div class="log-dialog-container">
      <!-- 标题栏 -->
      <div class="log-header">
        <VIcon icon="ri-terminal-box-fill" size="20" class="me-2 text-medium-emphasis" />
        <span class="text-body-1 font-weight-semibold">实时日志</span>
        <div style="flex: 1;" />
        <IconBtn size="small" @click="closeDialog">
          <VIcon icon="ri-close-line" />
        </IconBtn>
      </div>

      <!-- 过滤栏 -->
      <div class="log-toolbar">
        <div class="d-flex" style="gap: 4px;">
          <button
            v-for="f in filters"
            :key="f"
            class="filter-btn"
            :class="{ active: activeFilter === f }"
            @click="activeFilter = f"
          >
            {{ f }}
          </button>
        </div>
      </div>

      <!-- 日志内容 -->
      <div ref="logContainer" class="log-body">
        <div v-if="!filteredLogs.length" class="log-empty">
          <VIcon icon="ri-file-list-3-line" size="48" style="opacity: 0.2;" />
          <div style="margin-top: 8px; font-size: 0.875rem;">暂无日志</div>
        </div>

        <table v-else class="log-table">
          <tbody>
            <tr v-for="(log, i) in filteredLogs" :key="i" class="log-row">
              <td class="cell-level">
                <span class="level-tag" :class="`level-${log.level.toLowerCase()}`">
                  {{ log.level }}
                </span>
              </td>
              <td class="cell-time">{{ log.time }}</td>
              <td class="cell-msg">{{ log.message }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 底部状态栏 -->
      <div class="log-footer">
        <span class="footer-text">每 3 秒自动刷新</span>
        <div style="flex: 1;" />
        <button class="refresh-btn" :disabled="loading" @click="fetchLogs">
          <VIcon icon="ri-refresh-line" size="14" class="me-1" />
          立即刷新
        </button>
      </div>
    </div>
  </VDialog>
</template>

<style lang="scss" scoped>
.log-dialog-container {
  display: flex;
  flex-direction: column;
  width: 90vw;
  height: 82vh;
  background: rgb(var(--v-theme-surface));
  border-radius: 12px;
  overflow: hidden;
  color: rgba(var(--v-theme-on-surface), var(--v-high-emphasis-opacity));
}

.log-header {
  display: flex;
  align-items: center;
  padding: 14px 20px;
  border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}

.log-toolbar {
  display: flex;
  align-items: center;
  padding: 10px 20px;
  border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}

.filter-btn {
  padding: 4px 14px;
  border: none;
  border-radius: 6px;
  font-size: 0.8125rem;
  font-weight: 500;
  cursor: pointer;
  background: transparent;
  color: rgba(var(--v-theme-on-surface), 0.6);
  transition: background 0.15s, color 0.15s;

  &.active {
    background: rgb(var(--v-theme-primary));
    color: #fff;
  }
}

.refresh-btn {
  display: flex;
  align-items: center;
  padding: 4px 14px;
  border: none;
  border-radius: 6px;
  font-size: 0.8125rem;
  font-weight: 500;
  cursor: pointer;
  background: rgba(var(--v-theme-primary), 0.12);
  color: rgb(var(--v-theme-primary));

  &:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
}

.log-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  background: rgba(var(--v-theme-on-surface), 0.02);
}

.log-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: rgba(var(--v-theme-on-surface), 0.35);
}

.log-table {
  width: 100%;
  border-collapse: collapse;
  font-family: 'SF Mono', 'Cascadia Code', 'Fira Code', 'Consolas', monospace;
  font-size: 0.8125rem;
  line-height: 1.6;
}

.log-row {
  border-bottom: 1px solid rgba(var(--v-border-color), 0.06);
}

.cell-level {
  width: 90px;
  padding: 7px 12px;
  vertical-align: middle;
  white-space: nowrap;
}

.level-tag {
  display: inline-block;
  min-width: 68px;
  padding: 2px 0;
  border-radius: 4px;
  font-size: 0.6875rem;
  font-weight: 700;
  text-align: center;

  &.level-info {
    background: rgba(var(--v-theme-info), 0.15);
    color: rgb(var(--v-theme-info));
  }

  &.level-warning {
    background: rgba(var(--v-theme-warning), 0.15);
    color: rgb(var(--v-theme-warning));
  }

  &.level-error {
    background: rgba(var(--v-theme-error), 0.15);
    color: rgb(var(--v-theme-error));
  }
}

.cell-time {
  width: 110px;
  padding: 7px 8px;
  white-space: nowrap;
  color: rgba(var(--v-theme-on-surface), 0.4);
  font-size: 0.75rem;
  vertical-align: middle;
}

.cell-msg {
  padding: 7px 12px;
  word-break: break-all;
  vertical-align: middle;
}

.log-footer {
  display: flex;
  align-items: center;
  padding: 10px 20px;
  border-top: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}

.footer-text {
  font-size: 0.75rem;
  color: rgba(var(--v-theme-on-surface), 0.5);
}
</style>

<style lang="scss">
// 全局：让 VDialog overlay 内容居中且无动画
.log-dialog-overlay {
  transition: none !important;
  max-width: none !important;
  width: auto !important;

  .v-overlay__content {
    transition: none !important;
    max-width: none !important;
    width: auto !important;
  }
}
</style>

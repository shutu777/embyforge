<script setup>
import { onMounted } from 'vue'
import { useTheme } from 'vuetify'

const props = defineProps({
  themes: {
    type: Array,
    required: true,
  },
})

const {
  name: themeName,
  global: globalTheme,
} = useTheme()

// 从 localStorage 读取保存的主题
const savedTheme = localStorage.getItem('theme')
const initialTheme = savedTheme || themeName.value

const {
  state: currentThemeName,
  next: getNextThemeName,
  index: currentThemeIndex,
} = useCycleList(props.themes.map(t => t.name), { initialValue: initialTheme })

const changeTheme = () => {
  const nextTheme = getNextThemeName()

  globalTheme.name.value = nextTheme

  // 保存到 localStorage
  localStorage.setItem('theme', nextTheme)
}

// Update icon if theme is changed from other sources
watch(() => globalTheme.name.value, val => {
  currentThemeName.value = val

  // 保存到 localStorage
  localStorage.setItem('theme', val)
})

// 组件挂载时应用保存的主题
onMounted(() => {
  if (savedTheme) {
    globalTheme.name.value = savedTheme
  }
})
</script>

<template>
  <IconBtn @click="changeTheme">
    <VIcon :icon="props.themes[currentThemeIndex].icon" />
    <VTooltip
      activator="parent"
      open-delay="1000"
      scroll-strategy="close"
    >
      <span class="text-capitalize">{{ currentThemeName }}</span>
    </VTooltip>
  </IconBtn>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import VerticalNavSectionTitle from '@/@layouts/components/VerticalNavSectionTitle.vue'
import VerticalNavLink from '@layouts/components/VerticalNavLink.vue'

const { t } = useI18n()
</script>

<template>
  <!-- 👉 首页 -->
  <VerticalNavLink
    :item="{
      title: t('menu.dashboard'),
      icon: 'ri-home-smile-line',
      to: '/dashboard',
    }"
  />

  <!-- 👉 扫描工具 -->
  <VerticalNavSectionTitle
    :item="{
      heading: t('menu.scanTools'),
    }"
  />
  <VerticalNavLink
    :item="{
      title: t('menu.mediaScan'),
      icon: 'ri-radar-line',
      to: '/media-scan',
    }"
  />
  <VerticalNavLink
    :item="{
      title: t('menu.scrapeAnomaly'),
      icon: 'ri-file-search-line',
      to: '/scrape-anomaly',
    }"
  />
  <VerticalNavLink
    :item="{
      title: t('menu.duplicateMedia'),
      icon: 'ri-file-copy-2-line',
      to: '/duplicate-media',
    }"
  />
  <VerticalNavLink
    :item="{
      title: t('menu.episodeMapping'),
      icon: 'ri-git-branch-line',
      to: '/episode-mapping',
    }"
  />
  <VerticalNavLink
    :item="{
      title: t('menu.tmdbCache'),
      icon: 'ri-database-2-line',
      to: '/tmdb-cache',
    }"
  />

  <!-- 👉 Symedia管理 -->
  <VerticalNavSectionTitle
    :item="{
      heading: t('menu.symediaManagement'),
    }"
  />
  <VerticalNavLink
    :item="{
      title: t('menu.configRefresh'),
      icon: 'ri-refresh-line',
      to: '/symedia-config',
    }"
  />

  <!-- 👉 系统管理 -->
  <VerticalNavSectionTitle
    :item="{
      heading: t('menu.systemManagement'),
    }"
  />
  <VerticalNavLink
    :item="{
      title: t('menu.embyConfig'),
      icon: 'ri-settings-3-line',
      to: '/emby-config',
    }"
  />
  <VerticalNavLink
    :item="{
      title: t('menu.systemConfig'),
      icon: 'ri-tools-line',
      to: '/system-config',
    }"
  />
</template>

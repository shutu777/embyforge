<template>
  <VCard class="stats-card-modern">
    <VCardText>
      <div class="d-flex align-center justify-space-between mb-4">
        <div>
          <div class="text-caption text-medium-emphasis mb-1">
            {{ title }}
          </div>
          <div class="text-h4 font-weight-bold">
            {{ stats }}
          </div>
        </div>
        <VAvatar
          :color="color"
          variant="tonal"
          size="56"
          class="stats-icon-container"
        >
          <VIcon
            :icon="icon"
            size="32"
          />
        </VAvatar>
      </div>
      
      <div class="d-flex align-center">
        <VChip
          :color="change >= 0 ? 'success' : 'error'"
          size="small"
          variant="tonal"
          class="me-2"
        >
          <VIcon
            :icon="change >= 0 ? 'ri-arrow-up-line' : 'ri-arrow-down-line'"
            size="16"
            class="me-1"
          />
          {{ Math.abs(change) }}%
        </VChip>
        <span class="text-caption text-medium-emphasis">
          {{ subtitle }}
        </span>
      </div>
    </VCardText>
  </VCard>
</template>

<script setup>
defineProps({
  title: {
    type: String,
    required: true,
  },
  stats: {
    type: [String, Number],
    required: true,
  },
  change: {
    type: Number,
    default: 0,
  },
  subtitle: {
    type: String,
    default: '',
  },
  icon: {
    type: String,
    required: true,
  },
  color: {
    type: String,
    default: 'primary',
  },
})
</script>

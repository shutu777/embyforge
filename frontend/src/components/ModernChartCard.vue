<template>
  <VCard class="chart-container-modern">
    <VCardText>
      <div class="d-flex align-center justify-space-between mb-4">
        <div>
          <div class="text-h6 font-weight-semibold mb-1">
            {{ title }}
          </div>
          <div class="text-caption text-medium-emphasis">
            {{ subtitle }}
          </div>
        </div>
        
        <VBtnToggle
          v-if="filterOptions && filterOptions.length"
          v-model="selectedFilter"
          density="compact"
          variant="outlined"
          divided
          mandatory
        >
          <VBtn
            v-for="option in filterOptions"
            :key="option.value"
            :value="option.value"
            size="small"
          >
            {{ option.label }}
          </VBtn>
        </VBtnToggle>
      </div>
      
      <div class="chart-divider mb-4" />
      
      <div class="chart-content">
        <slot />
      </div>
    </VCardText>
  </VCard>
</template>

<script setup>
import { ref, watch } from 'vue'

const props = defineProps({
  title: {
    type: String,
    required: true,
  },
  subtitle: {
    type: String,
    default: '',
  },
  filterOptions: {
    type: Array,
    default: () => [],
  },
  defaultFilter: {
    type: String,
    default: null,
  },
})

const emit = defineEmits(['filterChange'])

const selectedFilter = ref(props.defaultFilter || (props.filterOptions[0]?.value || null))

watch(selectedFilter, newValue => {
  emit('filterChange', newValue)
})
</script>

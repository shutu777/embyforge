/* eslint-disable regex/invalid */
import { Icon } from '@iconify/vue'
import checkboxChecked from '@images/svg/checkbox-checked.svg'
import checkboxIndeterminate from '@images/svg/checkbox-indeterminate.svg'
import checkboxUnchecked from '@images/svg/checkbox-unchecked.svg'
import radioChecked from '@images/svg/radio-checked.svg'
import radioUnchecked from '@images/svg/radio-unchecked.svg'

const customIcons = {
  'mdi-checkbox-blank-outline': checkboxUnchecked,
  'mdi-checkbox-marked': checkboxChecked,
  'mdi-minus-box': checkboxIndeterminate,
  'mdi-radiobox-marked': radioChecked,
  'mdi-radiobox-blank': radioUnchecked,
}

const aliases = {
  info: 'ri-error-warning-line',
  success: 'ri-checkbox-circle-line',
  warning: 'ri-alert-line',
  error: 'ri-error-warning-line',
  calendar: 'ri-calendar-2-line',
  collapse: 'ri-arrow-up-s-line',
  complete: 'ri-check-line',
  cancel: 'ri-close-line',
  close: 'ri-close-line',
  delete: 'ri-close-circle-fill',
  clear: 'ri-close-line',
  prev: 'ri-arrow-left-s-line',
  next: 'ri-arrow-right-s-line',
  delimiter: 'ri-circle-line',
  sort: 'ri-arrow-up-line',
  expand: 'ri-arrow-down-s-line',
  menu: 'ri-menu-line',
  subgroup: 'ri-arrow-down-s-fill',
  dropdown: 'ri-arrow-down-s-line',
  edit: 'ri-pencil-line',
  ratingEmpty: 'ri-star-line',
  ratingFull: 'ri-star-fill',
  ratingHalf: 'ri-star-half-line',
  loading: 'ri-refresh-line',
  first: 'ri-skip-back-mini-line',
  last: 'ri-skip-forward-mini-line',
  unfold: 'ri-split-cells-vertical',
  file: 'ri-attachment-2',
  plus: 'ri-add-line',
  minus: 'ri-subtract-line',
  sortAsc: 'ri-arrow-up-line',
  sortDesc: 'ri-arrow-down-line',
}

/* eslint-enable */
export const iconify = {
  component: props => {
    // 优先加载自定义 SVG
    if (typeof props.icon === 'string') {
      const iconComponent = customIcons[props.icon]
      if (iconComponent)
        return h(iconComponent)
    }
    
    // 使用 Iconify Vue 组件按需加载图标
    // 这样只会加载实际使用的图标,而不是全部 1.5MB
    return h(Icon, {
      icon: props.icon,
      width: props.size || '1em',
      height: props.size || '1em',
    })
  },
}
export const icons = {
  defaultSet: 'iconify',
  aliases,
  sets: {
    iconify,
  },
}

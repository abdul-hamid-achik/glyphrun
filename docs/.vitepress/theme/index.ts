import type { Theme } from 'vitepress'
import DefaultTheme from 'vitepress/theme'
import SpecFrame from './SpecFrame.vue'
import './custom.css'

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component('SpecFrame', SpecFrame)
  },
} satisfies Theme

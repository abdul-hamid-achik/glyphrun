import type { Theme } from 'vitepress'
import DefaultTheme from 'vitepress/theme'
import SpecFrame from './SpecFrame.vue'
import './custom.css'
import { inject } from '@vercel/analytics'

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    if (typeof window !== 'undefined') {
      inject()
    }
    app.component('SpecFrame', SpecFrame)
  },
} satisfies Theme

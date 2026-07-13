import { defineConfig, type HeadConfig } from 'vitepress'

const SITE_URL = 'https://glyphrun.dev'
const OG_IMAGE = `${SITE_URL}/og.png`
const DEFAULT_DESCRIPTION =
  'Glyphrun is a terminal app testing framework: a local-first PTY test runner that drives TUIs and CLI workflows in a real pseudo-terminal, asserts against a deterministic virtual terminal emulator, and writes artifact packs built for humans and coding agents.'

export default defineConfig({
  title: 'Glyphrun',
  description: DEFAULT_DESCRIPTION,
  cleanUrls: true,
  lastUpdated: true,

  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg' }],
    ['link', { rel: 'icon', type: 'image/png', sizes: '32x32', href: '/favicon-32.png' }],
    ['link', { rel: 'apple-touch-icon', sizes: '180x180', href: '/apple-touch-icon.png' }],
    ['meta', { name: 'theme-color', content: '#D97706' }],
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
    [
      'script',
      { type: 'application/ld+json' },
      JSON.stringify({
        '@context': 'https://schema.org',
        '@type': 'SoftwareApplication',
        name: 'Glyphrun',
        applicationCategory: 'DeveloperApplication',
        operatingSystem: 'macOS, Linux, Windows',
        description: DEFAULT_DESCRIPTION,
        url: SITE_URL,
        license: 'https://opensource.org/licenses/MIT',
        offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
        author: { '@type': 'Person', name: 'Abdul Hamid Achik' },
      }),
    ],
    [
      'script',
      { type: 'application/ld+json' },
      JSON.stringify({
        '@context': 'https://schema.org',
        '@type': 'WebSite',
        name: 'Glyphrun',
        url: SITE_URL,
      }),
    ],
  ],

  sitemap: { hostname: SITE_URL },

  transformPageData(pageData) {
    const isHome = pageData.frontmatter.layout === 'home'
    const relPath = pageData.relativePath
      .replace(/(^|\/)index\.md$/, '$1')
      .replace(/\.md$/, '')
    const canonicalUrl = relPath ? `${SITE_URL}/${relPath}` : SITE_URL

    const description = pageData.frontmatter.description || DEFAULT_DESCRIPTION
    const title = isHome
      ? pageData.frontmatter.title || 'Glyphrun — Terminal & TUI Testing Framework'
      : `${pageData.title} | Glyphrun`

    const head: HeadConfig[] = [
      ['meta', { name: 'description', content: description }],
      ['link', { rel: 'canonical', href: canonicalUrl }],
      ['meta', { property: 'og:type', content: 'website' }],
      ['meta', { property: 'og:site_name', content: 'Glyphrun' }],
      ['meta', { property: 'og:title', content: title }],
      ['meta', { property: 'og:description', content: description }],
      ['meta', { property: 'og:url', content: canonicalUrl }],
      ['meta', { property: 'og:image', content: OG_IMAGE }],
      ['meta', { property: 'og:image:width', content: '1200' }],
      ['meta', { property: 'og:image:height', content: '630' }],
      ['meta', { name: 'twitter:title', content: title }],
      ['meta', { name: 'twitter:description', content: description }],
      ['meta', { name: 'twitter:image', content: OG_IMAGE }],
    ]

    pageData.frontmatter.head = [...(pageData.frontmatter.head ?? []), ...head]
  },

  themeConfig: {
    siteTitle: 'glyphrun',
    logo: '/logo.svg',
    nav: [
      { text: 'Guide', link: '/overview', activeMatch: '/overview' },
      { text: 'Authoring', link: '/authoring', activeMatch: '/authoring' },
      { text: 'Reference', link: '/commands', activeMatch: '/(commands|contract-hash|steps|verifiers|file-script-verifiers|count-verifier|artifacts|artifacts-pipeline|process-telemetry|configuration|redaction-block|distribution|github|mcp|snippets|troubleshooting|cairntrace-comparison|topics)' },
      { text: 'Agents', link: '/agents', activeMatch: '/agents' },
    ],

    sidebar: {
      '/': [
        {
          text: 'Getting Started',
          items: [
            { text: 'Overview', link: '/overview' },
            { text: 'Quickstart', link: '/quickstart' },
            { text: 'Concepts', link: '/authoring' },
          ],
        },
        {
          text: 'Reference',
          items: [
            { text: 'Commands', link: '/commands' },
            { text: 'Contract Hash', link: '/contract-hash' },
            { text: 'Steps', link: '/steps' },
            { text: 'Verifiers', link: '/verifiers' },
            { text: 'File & Script Verifiers', link: '/file-script-verifiers' },
            { text: 'Count Verifier', link: '/count-verifier' },
            { text: 'Artifacts', link: '/artifacts' },
            { text: 'Artifact Pipeline', link: '/artifacts-pipeline' },
            { text: 'Process Telemetry', link: '/process-telemetry' },
            { text: 'Configuration', link: '/configuration' },
            { text: 'Redaction', link: '/redaction-block' },
            { text: 'Snippets', link: '/snippets' },
            { text: 'Distribution', link: '/distribution' },
            { text: 'GitHub', link: '/github' },
            { text: 'MCP', link: '/mcp' },
            { text: 'Troubleshooting', link: '/troubleshooting' },
            { text: 'Cairntrace Comparison', link: '/cairntrace-comparison' },
            { text: 'Topics', link: '/topics' },
          ],
        },
        {
          text: 'For Agents',
          items: [
            { text: 'Agent Loop', link: '/agents' },
          ],
        },
      ],
    },

    socialLinks: [
      { icon: 'github', link: 'https://github.com/abdul-hamid-achik/glyphrun' },
    ],

    editLink: {
      pattern: 'https://github.com/abdul-hamid-achik/glyphrun/edit/main/docs/:path',
      text: 'Edit this page on GitHub',
    },

    footer: {
      message: 'Released under the MIT License.',
      copyright: 'Copyright © Abdul Hamid Achik',
    },

    search: { provider: 'local' },
  },
})

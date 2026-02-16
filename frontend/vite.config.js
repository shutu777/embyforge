import vue from '@vitejs/plugin-vue'
import vueJsx from '@vitejs/plugin-vue-jsx'
import { fileURLToPath } from 'node:url'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { defineConfig } from 'vite'
import vuetify from 'vite-plugin-vuetify'
import svgLoader from 'vite-svg-loader'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    vueJsx(),

    // Docs: https://github.com/vuetifyjs/vuetify-loader/tree/master/packages/vite-plugin
    vuetify({
      styles: {
        configFile: 'src/assets/styles/variables/_vuetify.scss',
      },
    }),
    Components({
      dirs: ['src/@core/components', 'src/components'],
      dts: true,
      resolvers: [
        componentName => {
          // Auto import `VueApexCharts`
          if (componentName === 'VueApexCharts')
            return { name: 'default', from: 'vue3-apexcharts', as: 'VueApexCharts' }
        },
      ],
    }),

    // Docs: https://github.com/antfu/unplugin-auto-import#unplugin-auto-import
    AutoImport({
      imports: ['vue', 'vue-router', '@vueuse/core', '@vueuse/math', 'pinia'],
      vueTemplate: true,

      // ℹ️ Disabled to avoid confusion & accidental usage
      ignore: ['useCookies', 'useStorage'],
      eslintrc: {
        enabled: true,
        filepath: './.eslintrc-auto-import.json',
      },
    }),
    svgLoader(),
  ],
  define: { 'process.env': {} },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
      '@core': fileURLToPath(new URL('./src/@core', import.meta.url)),
      '@layouts': fileURLToPath(new URL('./src/@layouts', import.meta.url)),
      '@images': fileURLToPath(new URL('./src/assets/images/', import.meta.url)),
      '@styles': fileURLToPath(new URL('./src/assets/styles/', import.meta.url)),
      '@configured-variables': fileURLToPath(new URL('./src/assets/styles/variables/_template.scss', import.meta.url)),
      'inter-ui': fileURLToPath(new URL('./node_modules/inter-ui', import.meta.url)),
    },
  },
  build: {
    target: 'es2015',
    minify: 'esbuild',
    cssCodeSplit: true,
    sourcemap: false,
    chunkSizeWarningLimit: 1000,
    rollupOptions: {
      output: {
        // 优化代码分割策略
        manualChunks: {
          // 将 Vue 核心库单独打包
          'vue-vendor': ['vue', 'vue-router', 'pinia'],

          // 将 Vuetify 单独打包
          'vuetify-vendor': ['vuetify'],

          // 将图表库单独打包（这是性能瓶颈）
          'charts-vendor': ['apexcharts', 'vue3-apexcharts'],

          // 将工具库单独打包
          'utils': ['@vueuse/core', '@vueuse/math'],
        },

        // 文件名包含哈希，用于缓存控制
        chunkFileNames: 'js/[name]-[hash].js',
        entryFileNames: 'js/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash].[ext]',
      },
    },
  },
  optimizeDeps: {
    exclude: ['vuetify'],
    entries: [
      './src/**/*.vue',
    ],

    // 预构建常用依赖，加快首次加载
    include: [
      'vue',
      'vue-router',
      'pinia',
      '@vueuse/core',
      '@vueuse/math',
      '@iconify/vue',
      'apexcharts',
      'vue3-apexcharts',
    ],
    force: false, // 仅在依赖变更时重新构建，启用缓存
    
    // 禁用预构建的 source map 以消除警告
    esbuildOptions: {
      sourcemap: false,
    },
  },
  server: {
    // API 代理，将 /api 请求转发到后端
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/uploads': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },

    // 开发服务器优化
    hmr: {
      overlay: false, // 禁用错误覆盖层，加快响应
    },

    // 预热常用文件
    warmup: {
      clientFiles: [
        './src/App.vue',
        './src/layouts/default.vue',
        './src/plugins/vuetify/index.js',
        './src/plugins/router/index.js',
      ],
    },
  },

  // CSS 优化
  css: {
    devSourcemap: false, // 禁用 CSS source map，加快构建
  },
})

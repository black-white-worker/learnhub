import { fileURLToPath, URL } from 'node:url'

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueDevTools from 'vite-plugin-vue-devtools'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    vueDevTools(),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    host: '0.0.0.0', // 监听所有地址（IPv4/IPv6），避免仅 IPv6 导致访问不了
    port: 5173, // 前端开发端口
    proxy: {
      // 凡是以 /api 开头的请求，转发到后端 go-zero 服务（8888）
      '/api': {
        target: 'http://localhost:8888',
        changeOrigin: true,
      },
    },
  },
})

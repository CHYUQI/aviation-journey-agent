import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 开发期只连真实后端（cmd/server，:8080）。
const apiTarget = 'http://localhost:8080'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/api': apiTarget,
    },
  },
})

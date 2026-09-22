import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// MOCK_API=1 → mock 服务（cmd/mockserver，:8081），用于前端独立联调；
// MOCK_API=0 或不设变量 → 真后端（:8080）。
const apiTarget = process.env.MOCK_API === '0'
  ? 'http://localhost:8081'
  : 'http://localhost:8080'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/api': apiTarget,
    },
  },
})

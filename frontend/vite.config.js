import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// In dev, forward API calls to the Go backend.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
      '/swagger': 'http://localhost:8080',
      '/openapi.yaml': 'http://localhost:8080',
    },
  },
})

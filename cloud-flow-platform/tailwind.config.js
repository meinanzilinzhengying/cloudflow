/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        dark: {
          100: '#1e1e2e',
          200: '#181825',
          300: '#11111b',
          400: '#0d0d14',
        },
        accent: {
          500: '#89b4fa',
          400: '#b4befe',
          300: '#f5c2e7',
        }
      }
    },
  },
  plugins: [],
}

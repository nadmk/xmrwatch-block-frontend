/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        brand: {
          50: '#f2f7f6',
          100: '#dbeeea',
          200: '#b6ddd6',
          300: '#88c3bb',
          400: '#5faca3',
          500: '#448f86',
          600: '#37736d',
          700: '#2f5d58',
          800: '#294c49',
          900: '#243f3d',
        },
      },
    },
  },
  plugins: [],
}

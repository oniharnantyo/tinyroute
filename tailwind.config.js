/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./internal/dashboard/**/*.templ",
    "./internal/dashboard/**/*_templ.go",
    "./internal/dashboard/**/*.go",
  ],
  theme: {
    extend: {},
  },
  plugins: [],
}

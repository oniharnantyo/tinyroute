/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: ["class"],
  content: [
    "./internal/dashboard/**/*.templ",
    "./internal/dashboard/**/*_templ.go",
    "./internal/dashboard/**/*.go",
  ],
  theme: {
    extend: {
      colors: {
        border: "var(--border)",
        input: "var(--input)",
        ring: "var(--ring)",
        background: "var(--background)",
        foreground: "var(--foreground)",
        primary: {
          DEFAULT: "var(--primary)",
          foreground: "var(--primary-foreground)",
        },
        secondary: {
          DEFAULT: "var(--secondary)",
          foreground: "var(--secondary-foreground)",
        },
        destructive: {
          DEFAULT: "var(--destructive)",
          foreground: "var(--destructive-foreground)",
        },
        muted: {
          DEFAULT: "var(--muted)",
          foreground: "var(--muted-foreground)",
        },
        accent: {
          DEFAULT: "var(--accent)",
          foreground: "var(--accent-foreground)",
        },
        popover: {
          DEFAULT: "var(--popover)",
          foreground: "var(--popover-foreground)",
        },
        card: {
          DEFAULT: "var(--card)",
          foreground: "var(--card-foreground)",
        },
      },
      borderRadius: {
        lg: "var(--radius)",
        md: "calc(var(--radius) - 2px)",
        sm: "calc(var(--radius) - 4px)",
      },
    },
  },
  plugins: [
    function({ addVariant }) {
      addVariant('data-open', ['&[data-open]', '&:where([data-state="open"])', '&:where([data-open]:not([data-open="false"]))']);
      addVariant('data-closed', ['&[data-closed]', '&:where([data-state="closed"])', '&:where([data-closed]:not([data-closed="false"]))']);
      addVariant('data-checked', ['&[data-checked]', '&:where([data-state="checked"])', '&:where([data-checked]:not([data-checked="false"]))']);
      addVariant('data-unchecked', ['&[data-unchecked]', '&:where([data-state="unchecked"])', '&:where([data-unchecked]:not([data-unchecked="false"]))']);
      addVariant('data-selected', ['&[data-selected]', '&:where([data-state="selected"])', '&:where([data-selected]:not([data-selected="false"]))']);
      addVariant('data-disabled', ['&[data-disabled]', '&:where([data-disabled]:not([data-disabled="false"]))', '&:where([aria-disabled="true"])']);
      addVariant('data-active', ['&[data-active]', '&:where([data-state="active"])', '&:where([data-active]:not([data-active="false"]))']);
      addVariant('data-current', ['&[data-current]', '&:where([data-current]:not([data-current="false"]))', '&:where([aria-current="page"])']);
      addVariant('data-highlighted', '&:where([data-highlighted]:not([data-highlighted="false"]))');
      addVariant('data-placeholder', '&:where([data-placeholder])');
      addVariant('supports-backdrop-filter', '@supports ((-webkit-backdrop-filter: none) or (backdrop-filter: none))');
    },
  ],
}

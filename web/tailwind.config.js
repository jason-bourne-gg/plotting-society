/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        // Shiv Rudra Group's brochure palette: deep olive, gold, cream.
        olive: {
          50: '#F5F7EF', 100: '#E8EDD9', 200: '#D1DBB4', 300: '#B2C287',
          400: '#94A862', 500: '#788C48', 600: '#5C6E37', 700: '#47562C',
          800: '#374327', 900: '#2C3520', 950: '#171D10',
        },
        gold: {
          50: '#FDF9ED', 100: '#F8EFCE', 200: '#F1DC9A', 300: '#E8C45F',
          400: '#DFAD35', 500: '#C9A227', 600: '#A8801E', 700: '#855F1C',
          800: '#6F4D1E', 900: '#5F411F', 950: '#37220E',
        },
        cream: '#FAF7EF',
        // The four sanctioned sectors, coloured as the layout plan colours them.
        sector: {
          1: '#E8913A', 2: '#4FA3DC', 3: '#57A55B', 4: '#8B7EC8',
        },
      },
      fontFamily: {
        display: ['"Fraunces"', 'Georgia', 'serif'],
        sans: ['"Plus Jakarta Sans"', 'system-ui', 'sans-serif'],
      },
      boxShadow: {
        card: '0 1px 2px rgba(23,29,16,.06), 0 8px 24px -12px rgba(23,29,16,.18)',
        lift: '0 2px 4px rgba(23,29,16,.08), 0 20px 40px -20px rgba(23,29,16,.35)',
      },
      keyframes: {
        'fade-up': {
          from: { opacity: '0', transform: 'translateY(8px)' },
          to: { opacity: '1', transform: 'translateY(0)' },
        },
        shimmer: {
          '100%': { transform: 'translateX(100%)' },
        },
      },
      animation: {
        'fade-up': 'fade-up .4s cubic-bezier(.16,1,.3,1) both',
        shimmer: 'shimmer 1.6s infinite',
      },
    },
  },
  plugins: [],
}

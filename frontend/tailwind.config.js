/** @type {import('tailwindcss').Config} */
const { nextui } = require("@nextui-org/react");

module.exports = {
  content: [
    './src/pages/**/*.{js,ts,jsx,tsx,mdx}',
    './src/components/**/*.{js,ts,jsx,tsx,mdx}',
    './src/app/**/*.{js,ts,jsx,tsx,mdx}',
    './node_modules/@nextui-org/theme/dist/**/*.{js,ts,jsx,tsx}'
  ],
  theme: {
    extend: {
      keyframes: {
        slideFromRight: {
          '0%': { transform: 'translateX(100%)', opacity: 0 },
          '100%': { transform: 'translateX(0)', opacity: 1 },
        },
        slideToLeft: {
          '0%': { transform: 'translateX(0)', opacity: 1 },
          '100%': { transform: 'translateX(-100%)', opacity: 0 },
        },
        slideFromLeft: {
          '0%': { transform: 'translateX(-100%)', opacity: 0 },
          '100%': { transform: 'translateX(0)', opacity: 1 },
        },
        slideToRight: {
          '0%': { transform: 'translateX(0)', opacity: 1 },
          '100%': { transform: 'translateX(100%)', opacity: 0 },
        },
        gradient: {
          '0%, 100%': {
            'background-position': '0% 50%',
          },
          '50%': {
            'background-position': '100% 50%',
          },
        },
        dotMove: {
          '0%': { 
            top: '-6px',
            left: '0%',
            transform: 'translateX(-50%)'
          },
          '25%': { 
            top: '-6px',
            left: '100%',
            transform: 'translateX(-50%)'
          },
          '35%': { 
            top: '-6px',
            left: '100%',
            transform: 'translateX(-50%)'
          },
          '40%': { 
            top: '0%',
            left: 'calc(100% + 6px)',
            transform: 'translateY(-50%)'
          },
          '60%': { 
            top: '100%',
            left: 'calc(100% + 6px)',
            transform: 'translateY(-50%)'
          },
          '65%': { 
            top: '100%',
            left: '100%',
            transform: 'translate(-50%, 6px)'
          },
          '85%': { 
            top: '100%',
            left: '0%',
            transform: 'translate(-50%, 6px)'
          },
          '90%': { 
            top: '100%',
            left: '0%',
            transform: 'translate(-50%, 6px)'
          },
          '95%': { 
            top: '0%',
            left: '-6px',
            transform: 'translateY(-50%)'
          },
          '100%': { 
            top: '-6px',
            left: '0%',
            transform: 'translateX(-50%)'
          }
        },
        pulseSlow: {
          '0%, 100%': { 
            opacity: 0.4,
            transform: 'scale(1)'
          },
          '50%': { 
            opacity: 0.8,
            transform: 'scale(1.03)'
          }
        },
        pulseShadow: {
          '0%, 100%': { 
            'box-shadow': '0 0 0 0 rgba(var(--color-primary), 0.7)'
          },
          '50%': { 
            'box-shadow': '0 0 30px 10px rgba(var(--color-primary), 0.4)'
          }
        },
        borderGlow: {
          '0%, 100%': { opacity: 1 },
          '50%': { opacity: 0.5 },
        },
        borderGradient: {
          '0%': {
            transform: 'translateX(-50%) rotate(0deg)',
          },
          '100%': {
            transform: 'translateX(0%) rotate(360deg)',
          },
        },
      },
      animation: {
        'slideFromRight': 'slideFromRight 0.5s ease-out forwards',
        'slideToLeft': 'slideToLeft 0.5s ease-out forwards',
        'slideFromLeft': 'slideFromLeft 0.5s ease-out forwards',
        'slideToRight': 'slideToRight 0.5s ease-out forwards',
        'gradient': 'gradient 6s ease infinite',
        'dot-move': 'dotMove 6s cubic-bezier(0.4, 0, 0.2, 1) infinite',
        'pulse-slow': 'pulseSlow 2s ease-in-out infinite',
        'pulse-shadow': 'pulseShadow 1.5s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        'border-move': 'borderGlow 2s ease-in-out infinite',
        'border-gradient': 'borderGradient 8s linear infinite',
      },
      backgroundSize: {
        'gradient-size': '200% 200%',
      },
    },
  },
  darkMode: "class",
  plugins: [nextui()],
}

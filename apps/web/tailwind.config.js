module.exports = {
  content: ["./src/app/**/*.{js,ts,jsx,tsx}", "./src/components/**/*.{js,ts,jsx,tsx}"],
  theme: {
    extend: {
      backgroundImage: {
        aero:
          "radial-gradient(1200px 800px at 20% -10%, rgba(120,200,255,0.75), transparent 50%), radial-gradient(900px 700px at 100% 0%, rgba(94,234,212,0.55), transparent 45%), linear-gradient(135deg, #e8f5ff 0%, #c7ecff 35%, #a5d8ff 70%, #8fbfff 100%)",
      },
      fontFamily: { sans: ["'Segoe UI Variable'", "Segoe UI", "Helvetica Neue", "Arial", "sans-serif"] },
    },
  },
  plugins: [],
};

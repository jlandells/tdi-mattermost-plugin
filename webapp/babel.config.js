module.exports = {
  presets: [
    ['@babel/preset-env', { targets: { chrome: 66 }, modules: false }],
    ['@babel/preset-react', { runtime: 'automatic' }],
  ],
};

const path = require('path');
const PLUGIN_ID = require('../plugin.json').id;

module.exports = {
  entry: ['./src/index.jsx'],
  resolve: {
    modules: ['src', 'node_modules'],
    extensions: ['.js', '.jsx'],
  },
  module: {
    rules: [
      {
        test: /\.(js|jsx)$/,
        exclude: /node_modules/,
        use: { loader: 'babel-loader' },
      },
      {
        test: /\.css$/,
        use: ['style-loader', 'css-loader'],
      },
    ],
  },
  externals: {
    react: 'React',
    'react-dom': 'ReactDOM',
    redux: 'Redux',
    'react-redux': 'ReactRedux',
  },
  output: {
    path: path.join(__dirname, '/dist'),
    publicPath: '/',
    filename: 'main.js',
    devtoolNamespace: PLUGIN_ID,
  },
  mode: 'production',
};

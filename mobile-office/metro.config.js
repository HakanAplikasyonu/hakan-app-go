// Metro: ortak paket (packages/mobile-core, #295) uygulamanın dışında durur.
// İzlenen klasörlere eklenir; paketin içe aktardığı modüller (react,
// react-native, expo-*) bu uygulamanın node_modules'ından çözülür, böylece
// tek kopya kalır.
const path = require('path');
const { getDefaultConfig } = require('expo/metro-config');

const config = getDefaultConfig(__dirname);
const core = path.resolve(__dirname, '../packages/mobile-core');

config.watchFolders = [...(config.watchFolders ?? []), core];
config.resolver.nodeModulesPaths = [path.resolve(__dirname, 'node_modules'), ...(config.resolver.nodeModulesPaths ?? [])];

module.exports = config;

/**
 * plugins/webfontloader.js
 *
 * webfontloader documentation: https://github.com/typekit/webfontloader
 */

// 完全禁用 webfontloader，字体已在 index.html 中预加载
export default function () {
  if (import.meta.env.MODE !== 'production') {
    console.log('[webfontloader] 跳过字体加载，使用 index.html 中的预加载')
  }
}

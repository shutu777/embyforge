/**
 * 资源加载器
 * 管理资源的加载优先级和策略
 */
class ResourceLoader {
  constructor() {
    this.loadedResources = new Set()
  }

  /**
   * 预连接到外部域名
   * @param {string[]} domains - 域名列表
   */
  preconnectDomains(domains) {
    domains.forEach(domain => {
      const link = document.createElement('link')

      link.rel = 'preconnect'
      link.href = domain
      link.crossOrigin = 'anonymous'
      document.head.appendChild(link)
    })
  }

  /**
   * 预加载关键资源
   * @param {string} url - 资源 URL
   * @param {string} as - 资源类型 (script, style, font, image)
   * @param {string|null} type - MIME 类型
   */
  preloadResource(url, as, type = null) {
    if (this.loadedResources.has(url))
      return

    const link = document.createElement('link')

    link.rel = 'preload'
    link.href = url
    link.as = as
    if (type)
      link.type = type

    document.head.appendChild(link)
    this.loadedResources.add(url)
  }

  /**
   * 延迟加载字体
   * @param {Array<{family: string, url: string, weight: string|number, style: string, display: string}>} fonts - 字体配置列表
   * @returns {Promise<void[]>}
   */
  async loadFonts(fonts) {
    const fontPromises = fonts.map(font => {
      return new FontFace(
        font.family,
        `url(${font.url})`,
        {
          weight: font.weight,
          style: font.style,
          display: font.display || 'swap',
        },
      ).load().then(loadedFont => {
        document.fonts.add(loadedFont)
      })
    })

    return Promise.all(fontPromises)
  }

  /**
   * 延迟加载脚本
   * @param {string} url - 脚本 URL
   * @returns {Promise<void>}
   */
  async loadScript(url) {
    if (this.loadedResources.has(url))
      return Promise.resolve()

    return new Promise((resolve, reject) => {
      const script = document.createElement('script')

      script.src = url
      script.async = true
      script.onload = () => {
        this.loadedResources.add(url)
        resolve()
      }
      script.onerror = reject
      document.body.appendChild(script)
    })
  }

  /**
   * 延迟加载样式
   * @param {string} url - 样式 URL
   * @returns {Promise<void>}
   */
  async loadStyle(url) {
    if (this.loadedResources.has(url))
      return Promise.resolve()

    return new Promise((resolve, reject) => {
      const link = document.createElement('link')

      link.rel = 'stylesheet'
      link.href = url
      link.onload = () => {
        this.loadedResources.add(url)
        resolve()
      }
      link.onerror = reject
      document.head.appendChild(link)
    })
  }
}

// 导出单例
export const resourceLoader = new ResourceLoader()

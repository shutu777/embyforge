/**
 * 性能监控器
 * 监控和报告性能指标
 */
class PerformanceMonitor {
  constructor() {
    this.metrics = {}
    this.thresholds = {
      FCP: 1800, // First Contentful Paint
      LCP: 2500, // Largest Contentful Paint
      FID: 100, // First Input Delay (deprecated in web-vitals v5)
      INP: 200, // Interaction to Next Paint (replaces FID in web-vitals v5)
      CLS: 0.1, // Cumulative Layout Shift
      TTI: 3800, // Time to Interactive
      TTFB: 600, // Time to First Byte
    }
  }

  /**
   * 初始化监控
   */
  init() {
    this.measureWebVitals()
    this.measureCustomMetrics()
    this.setupPerformanceObserver()
  }

  /**
   * 监控 Web Vitals
   * 需要安装 web-vitals 库: npm install web-vitals
   */
  measureWebVitals() {
    // 动态导入 web-vitals，避免阻塞主线程
    import('web-vitals').then((webVitals) => {
      const { onCLS, onFCP, onLCP, onTTFB, onINP, onFID } = webVitals
      
      onFCP(metric => this.handleMetric('FCP', metric))
      onLCP(metric => this.handleMetric('LCP', metric))
      onCLS(metric => this.handleMetric('CLS', metric))
      onTTFB(metric => this.handleMetric('TTFB', metric))
      
      // web-vitals v5+ 使用 onINP 替代 onFID
      if (onINP) {
        onINP(metric => this.handleMetric('INP', metric))
      } else if (onFID) {
        onFID(metric => this.handleMetric('FID', metric))
      }
    }).catch(err => {
      if (import.meta.env.MODE !== 'production') {
        console.warn('[Performance] web-vitals 加载失败，使用降级方案:', err)
      }
      this.useFallbackMeasurement()
    })
  }

  /**
   * 处理指标
   * @param {string} name - 指标名称
   * @param {object} metric - 指标对象
   */
  handleMetric(name, metric) {
    const value = metric.value

    this.metrics[name] = value

    if (import.meta.env.MODE !== 'production') {
      console.log(`[Performance] ${name}: ${value.toFixed(2)}${name === 'CLS' ? '' : 'ms'}`)
    }

    // 检查是否超过阈值
    if (this.thresholds[name] && value > this.thresholds[name]) {
      if (import.meta.env.MODE !== 'production') {
        console.warn(
          `[Performance Warning] ${name} (${value.toFixed(2)}) exceeds threshold (${this.thresholds[name]})`,
        )
      }
    }

    // 上报数据（可选）
    this.reportMetric(name, value)
  }

  /**
   * 自定义指标监控
   */
  measureCustomMetrics() {
    // 测量 Vue 初始化时间
    performance.mark('vue-init-start')

    // 在 Vue mounted 后调用
    window.addEventListener('vue-mounted', () => {
      performance.mark('vue-init-end')
      performance.measure('vue-init', 'vue-init-start', 'vue-init-end')

      const measure = performance.getEntriesByName('vue-init')[0]
      if (measure) {
        this.handleMetric('VueInit', { value: measure.duration })
      }
    })
  }

  /**
   * 设置 Performance Observer
   */
  setupPerformanceObserver() {
    if (!('PerformanceObserver' in window))
      return

    try {
      // 监控资源加载
      const resourceObserver = new PerformanceObserver(list => {
        list.getEntries().forEach(entry => {
          if (entry.duration > 1000) {
            if (import.meta.env.MODE !== 'production') {
              console.warn(`[Performance] Slow resource: ${entry.name} (${entry.duration.toFixed(2)}ms)`)
            }
          }
        })
      })

      resourceObserver.observe({ entryTypes: ['resource'] })

      // 监控长任务
      try {
        const longTaskObserver = new PerformanceObserver(list => {
          list.getEntries().forEach(entry => {
            if (import.meta.env.MODE !== 'production') {
              console.warn(`[Performance] Long task detected: ${entry.duration.toFixed(2)}ms`)
            }
          })
        })

        longTaskObserver.observe({ entryTypes: ['longtask'] })
      }
      catch (e) {
        // longtask 可能不被支持
        if (import.meta.env.MODE !== 'production') {
          console.debug('[Performance] longtask observer not supported')
        }
      }
    }
    catch (error) {
      if (import.meta.env.MODE !== 'production') {
        console.warn('[Performance] PerformanceObserver setup failed:', error)
      }
    }
  }

  /**
   * 上报指标
   * @param {string} name - 指标名称
   * @param {number} value - 指标值
   */
  reportMetric(name, value) {
    // 可以发送到分析服务
    if (window.gtag) {
      window.gtag('event', 'performance_metric', {
        metricName: name,
        metricValue: value,
        pagePath: window.location.pathname,
      })
    }
  }

  /**
   * 获取所有指标
   * @returns {object} 所有指标
   */
  getAllMetrics() {
    return { ...this.metrics }
  }

  /**
   * 生成性能报告
   * @returns {object} 性能报告
   */
  generateReport() {
    const navigation = performance.getEntriesByType('navigation')[0]
    const resources = performance.getEntriesByType('resource')

    if (!navigation) {
      return {
        vitals: this.metrics,
        navigation: null,
        resources: {
          total: resources.length,
          totalSize: resources.reduce((sum, r) => sum + (r.transferSize || 0), 0),
          byType: this.groupResourcesByType(resources),
        },
      }
    }

    return {
      vitals: this.metrics,
      navigation: {
        dns: navigation.domainLookupEnd - navigation.domainLookupStart,
        tcp: navigation.connectEnd - navigation.connectStart,
        request: navigation.responseStart - navigation.requestStart,
        response: navigation.responseEnd - navigation.responseStart,
        domParsing: navigation.domInteractive - navigation.responseEnd,
        domContentLoaded: navigation.domContentLoadedEventEnd - navigation.domContentLoadedEventStart,
        load: navigation.loadEventEnd - navigation.loadEventStart,
      },
      resources: {
        total: resources.length,
        totalSize: resources.reduce((sum, r) => sum + (r.transferSize || 0), 0),
        byType: this.groupResourcesByType(resources),
      },
    }
  }

  /**
   * 按类型分组资源
   * @param {PerformanceResourceTiming[]} resources - 资源列表
   * @returns {object} 分组后的资源
   */
  groupResourcesByType(resources) {
    return resources.reduce((acc, resource) => {
      const type = this.getResourceType(resource.name)
      if (!acc[type]) {
        acc[type] = { count: 0, size: 0 }
      }
      acc[type].count++
      acc[type].size += resource.transferSize || 0
      
      return acc
    }, {})
  }

  /**
   * 获取资源类型
   * @param {string} url - 资源 URL
   * @returns {string} 资源类型
   */
  getResourceType(url) {
    if (url.endsWith('.js'))
      return 'script'
    if (url.endsWith('.css'))
      return 'style'
    if (/\.(?:png|jpg|jpeg|gif|svg|webp)$/.test(url))
      return 'image'
    if (/\.(?:woff2?|ttf|eot)$/.test(url))
      return 'font'
    
    return 'other'
  }

  /**
   * 降级方案：使用 Navigation Timing API
   */
  useFallbackMeasurement() {
    window.addEventListener('load', () => {
      const navigation = performance.getEntriesByType('navigation')[0]
      if (navigation) {
        this.metrics.loadTime = navigation.loadEventEnd - navigation.fetchStart
        if (import.meta.env.MODE !== 'production') {
          console.log('[Performance] Fallback load time:', this.metrics.loadTime)
        }
      }
    })
  }
}

// 导出单例
export const performanceMonitor = new PerformanceMonitor()

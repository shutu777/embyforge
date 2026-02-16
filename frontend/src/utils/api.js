import axios from 'axios'

const api = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api',
  timeout: 300000, // 扫描接口可能耗时较长，设置 5 分钟超时
})

// 请求拦截器：注入 JWT 令牌
api.interceptors.request.use(config => {
  const token = localStorage.getItem('token')
  if (token)
    config.headers.Authorization = `Bearer ${token}`

  return config
})

// 响应拦截器：处理 401 未认证响应
api.interceptors.response.use(
  response => response,
  error => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      window.location.href = '/login'
    }

    return Promise.reject(error)
  },
)

export default api

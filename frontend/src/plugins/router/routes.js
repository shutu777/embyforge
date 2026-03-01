export const routes = [
  { path: '/', redirect: '/dashboard' },
  {
    path: '/',
    component: () => import('@/layouts/default.vue'),
    children: [
      {
        path: 'dashboard',
        name: 'dashboard',
        component: () => import(/* webpackPrefetch: true */ '@/pages/dashboard.vue'),
      },
      {
        path: 'emby-config',
        name: 'emby-config',
        component: () => import('@/pages/emby-config.vue'),
      },
      {
        path: 'media-scan',
        name: 'media-scan',
        component: () => import('@/pages/media-scan.vue'),
      },
      {
        path: 'scrape-anomaly',
        name: 'scrape-anomaly',
        component: () => import('@/pages/scrape-anomaly.vue'),
      },
      {
        path: 'duplicate-media',
        name: 'duplicate-media',
        component: () => import('@/pages/duplicate-media.vue'),
      },
      {
        path: 'episode-mapping',
        name: 'episode-mapping',
        component: () => import('@/pages/episode-mapping.vue'),
      },
      {
        path: 'tmdb-cache',
        name: 'tmdb-cache',
        component: () => import('@/pages/tmdb-cache.vue'),
      },
      {
        path: 'emby-cache',
        name: 'emby-cache',
        component: () => import('@/pages/emby-cache.vue'),
      },
      {
        path: 'media-query',
        name: 'media-query',
        component: () => import('@/pages/media-query.vue'),
      },
      {
        path: 'hdhive-search',
        name: 'hdhive-search',
        component: () => import('@/pages/hdhive-search.vue'),
      },
      {
        path: 'system-config',
        name: 'system-config',
        component: () => import('@/pages/system-config.vue'),
      },
      {
        path: 'symedia-config',
        name: 'symedia-config',
        component: () => import('@/pages/symedia-config.vue'),
      },
      {
        path: 'hdhive-config',
        name: 'hdhive-config',
        component: () => import('@/pages/hdhive-config.vue'),
      },
      {
        path: 'pan115-config',
        name: 'pan115-config',
        component: () => import('@/pages/pan115-config.vue'),
      },
      {
        path: 'rendering-words',
        name: 'rendering-words',
        component: () => import('@/pages/rendering-words.vue'),
      },
      {
        path: 'profile',
        name: 'profile',
        component: () => import('@/pages/profile.vue'),
      },
    ],
  },
  {
    path: '/',
    component: () => import('@/layouts/blank.vue'),
    children: [
      {
        path: 'login',
        name: 'login',
        component: () => import('@/pages/login.vue'),
      },
      {
        path: '/:pathMatch(.*)*',
        component: () => import('@/pages/[...error].vue'),
      },
    ],
  },
]

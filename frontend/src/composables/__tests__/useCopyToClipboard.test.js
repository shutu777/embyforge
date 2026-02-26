import { describe, it, expect, vi, beforeEach } from 'vitest'
import * as fc from 'fast-check'

// mock useClipboard 和 useSnackbar
const mockCopy = vi.fn()
const mockSuccess = vi.fn()
const mockError = vi.fn()

vi.mock('@vueuse/core', () => ({
  useClipboard: () => ({ copy: mockCopy }),
}))

vi.mock('@/composables/useSnackbar', () => ({
  useSnackbar: () => ({
    success: mockSuccess,
    error: mockError,
  }),
}))

import { useCopyToClipboard } from '../useCopyToClipboard'

describe('useCopyToClipboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // Feature: click-to-copy, Property 1: copyText 成功时正确传递文本并提示
  // Validates: Requirements 1.1, 1.2, 1.3, 2.2
  it('Property 1: 对于任意非空字符串，成功时应将原文传递给 copy 并显示包含 label 的成功提示', async () => {
    await fc.assert(
      fc.asyncProperty(
        fc.string({ minLength: 1 }),
        fc.string({ minLength: 1 }),
        async (text, label) => {
          vi.clearAllMocks()
          mockCopy.mockResolvedValueOnce(undefined)

          const { copyText } = useCopyToClipboard()
          await copyText(text, label)

          // (a) text 原样传递给 copy
          expect(mockCopy).toHaveBeenCalledWith(text)
          // (b) 成功提示包含 label
          expect(mockSuccess).toHaveBeenCalledTimes(1)
          const msg = mockSuccess.mock.calls[0][0]
          expect(msg).toContain(label)
          // 未调用 error
          expect(mockError).not.toHaveBeenCalled()
        },
      ),
      { numRuns: 100 },
    )
  })

  // Feature: click-to-copy, Property 2: copyText 失败时显示错误提示
  // Validates: Requirements 4.1
  it('Property 2: 对于任意非空字符串，当 copy 抛出异常时应显示错误提示', async () => {
    await fc.assert(
      fc.asyncProperty(
        fc.string({ minLength: 1 }),
        fc.string({ minLength: 1 }),
        async (text, label) => {
          vi.clearAllMocks()
          mockCopy.mockRejectedValueOnce(new Error('clipboard error'))

          const { copyText } = useCopyToClipboard()
          await copyText(text, label)

          // 应调用 error
          expect(mockError).toHaveBeenCalledTimes(1)
          expect(mockError).toHaveBeenCalledWith('复制失败')
          // 未调用 success
          expect(mockSuccess).not.toHaveBeenCalled()
        },
      ),
      { numRuns: 100 },
    )
  })
})

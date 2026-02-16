package workerpool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Config Worker Pool 配置
type Config struct {
	MinWorkers  int           // 最小 worker 数量（空闲时保留）
	MaxWorkers  int           // 最大 worker 数量（繁忙时上限）
	IdleTimeout time.Duration // worker 空闲超时后回收
}

// Result 包装任务执行结果
type Result[T any] struct {
	Value T
	Err   error
}

// Pool 动态泛型 Worker Pool
type Pool[T any] struct {
	config      Config
	tasks       chan func() Result[T]
	results     []Result[T]
	resultsMu   sync.Mutex
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	activeCount atomic.Int32 // 当前活跃 worker 数
	submitted   atomic.Int32 // 已提交任务数
	workerMu    sync.Mutex   // 保护 worker 创建逻辑
	closed      atomic.Bool  // 标记是否已关闭任务 channel
}

// New 创建动态 Worker Pool，启动 MinWorkers 个初始 worker
func New[T any](ctx context.Context, config Config) *Pool[T] {
	if config.MinWorkers < 1 {
		config.MinWorkers = 1
	}
	if config.MaxWorkers < config.MinWorkers {
		config.MaxWorkers = config.MinWorkers
	}
	if config.IdleTimeout <= 0 {
		config.IdleTimeout = 30 * time.Second
	}

	poolCtx, cancel := context.WithCancel(ctx)

	p := &Pool[T]{
		config:  config,
		tasks:   make(chan func() Result[T], config.MaxWorkers),
		results: make([]Result[T], 0),
		ctx:     poolCtx,
		cancel:  cancel,
	}

	// 启动最小数量的 worker
	for i := 0; i < config.MinWorkers; i++ {
		p.startWorker(true)
	}

	return p
}

// startWorker 启动一个 worker goroutine
// isCore 标记是否为核心 worker（核心 worker 不会因空闲超时退出）
func (p *Pool[T]) startWorker(isCore bool) {
	p.activeCount.Add(1)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer p.activeCount.Add(-1)
		p.workerLoop(isCore)
	}()
}

// workerLoop 是 worker 的主循环
func (p *Pool[T]) workerLoop(isCore bool) {
	for {
		if isCore {
			// 核心 worker：阻塞等待任务或 context 取消
			select {
			case task, ok := <-p.tasks:
				if !ok {
					return
				}
				p.executeTask(task)
			case <-p.ctx.Done():
				// context 取消后，继续处理 channel 中剩余的任务
				for {
					select {
					case task, ok := <-p.tasks:
						if !ok {
							return
						}
						p.executeTask(task)
					default:
						return
					}
				}
			}
		} else {
			// 非核心 worker：空闲超时后退出
			select {
			case task, ok := <-p.tasks:
				if !ok {
					return
				}
				p.executeTask(task)
			case <-time.After(p.config.IdleTimeout):
				return
			case <-p.ctx.Done():
				// context 取消后，处理剩余任务
				for {
					select {
					case task, ok := <-p.tasks:
						if !ok {
							return
						}
						p.executeTask(task)
					default:
						return
					}
				}
			}
		}
	}
}

// executeTask 执行单个任务，捕获 panic
func (p *Pool[T]) executeTask(task func() Result[T]) {
	var result Result[T]
	func() {
		defer func() {
			if r := recover(); r != nil {
				result = Result[T]{
					Err: fmt.Errorf("worker panic: %v", r),
				}
			}
		}()
		result = task()
	}()

	p.resultsMu.Lock()
	p.results = append(p.results, result)
	p.resultsMu.Unlock()
}

// Submit 提交任务到 Worker Pool
// 如果所有 worker 繁忙且未达 MaxWorkers 则自动扩展
// 在 context 取消时不阻塞
func (p *Pool[T]) Submit(task func() Result[T]) {
	// context 已取消，不接受新任务
	select {
	case <-p.ctx.Done():
		return
	default:
	}

	p.submitted.Add(1)

	// 尝试非阻塞发送
	select {
	case p.tasks <- task:
		return
	default:
	}

	// channel 满了，尝试扩展 worker
	p.workerMu.Lock()
	if int(p.activeCount.Load()) < p.config.MaxWorkers {
		p.startWorker(false)
	}
	p.workerMu.Unlock()

	// 阻塞发送，但在 context 取消时退出
	select {
	case p.tasks <- task:
	case <-p.ctx.Done():
	}
}

// Wait 关闭任务 channel，等待所有 worker 完成，返回结果
func (p *Pool[T]) Wait() []Result[T] {
	if p.closed.CompareAndSwap(false, true) {
		close(p.tasks)
	}
	p.wg.Wait()

	p.resultsMu.Lock()
	defer p.resultsMu.Unlock()
	results := make([]Result[T], len(p.results))
	copy(results, p.results)
	return results
}

// ActiveWorkers 返回当前活跃 worker 数量
func (p *Pool[T]) ActiveWorkers() int {
	return int(p.activeCount.Load())
}

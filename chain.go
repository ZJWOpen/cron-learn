package cron

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

// JobWrapper用某种行为装饰给定的Job。
type JobWrapper func(Job) Job

// Chain是JobWrappers的序列，
// 它使用诸如日志记录或同步之类的跨领域行为来修饰提交的作业。
type Chain struct {
	wrappers []JobWrapper
}

// NewChain返回由给定JobWrappers组成的Chain。
func NewChain(c ...JobWrapper) Chain {
	return Chain{c}
}

// Then方法 用链中的所有JobWrappers装饰给定的作业。
//
// 下面的形式:
//     NewChain(m1, m2, m3).Then(job)
// 与下面的形式等效:
//     m1(m2(m3(job)))
func (c Chain) Then(j Job) Job {
	for i := range c.wrappers {
		j = c.wrappers[len(c.wrappers)-i-1](j)
	}
	return j
}

// 恢复job中的异常，并将他们打印到给定的日志器中
func Recover(logger Logger) JobWrapper {
	return func(j Job) Job {
		return FuncJob(func() {
			defer func() {
				if r := recover(); r != nil {
					const size = 64 << 10
					buf := make([]byte, size)
					buf = buf[:runtime.Stack(buf, false)]
					err, ok := r.(error)
					if !ok {
						err = fmt.Errorf("%v", r)
					}
					logger.Error(err, "panic", "stack", "...\n"+string(buf))
				}
			}()
			j.Run()
		})
	}
}

// DelayIfStillRunning序列化作业，将后续运行延迟到上一个运行完成为止。
// 延迟超过一分钟后运行的作业会将延迟记录在信息中。
func DelayIfStillRunning(logger Logger) JobWrapper {
	return func(j Job) Job {
		var mu sync.Mutex
		return FuncJob(func() {
			start := time.Now()
			mu.Lock()
			defer mu.Unlock()
			if dur := time.Since(start); dur > time.Minute {
				logger.Info("delay", "duration", dur)
			}
			j.Run()
		})
	}
}

// 如果先前的调用仍在运行，则SkipIfStillRunning跳过对Job的调用。
// 它将会在Info级别上打印skip在给定的日志器上
func SkipIfStillRunning(logger Logger) JobWrapper {
	return func(j Job) Job {
		var ch = make(chan struct{}, 1)
		ch <- struct{}{}
		return FuncJob(func() {
			select {
			case v := <-ch:
				j.Run()
				ch <- v
			default:
				logger.Info("skip")
			}
		})
	}
}

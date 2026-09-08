package pool

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Task struct {
	ID   int64
	Func func() error
}

type Pool struct {
	Tasks       chan Task
	NumWorkers  int
	WorkerGroup sync.WaitGroup
	errMu       sync.Mutex
	errs        []error
}

func NewPool(numWorkers int) *Pool {
	return &Pool{
		Tasks:      make(chan Task, 1000),
		NumWorkers: numWorkers,
	}
}

func (p *Pool) Start() {
	for i := 0; i < p.NumWorkers; i++ {
		p.WorkerGroup.Add(1)
		go func() {
			defer p.WorkerGroup.Done()
			for task := range p.Tasks {
				if err := task.Func(); err != nil {
					if errors.Is(err, context.Canceled) {
						continue
					}
					p.errMu.Lock()
					p.errs = append(p.errs, err)
					p.errMu.Unlock()
				}
			}
		}()
	}
}

func (p *Pool) Stop() error {
	close(p.Tasks)
	p.WorkerGroup.Wait()
	return errors.Join(p.errs...)
}

func (p *Pool) Submit(f func() error) {
	t := Task{
		Func: f,
		ID:   time.Now().Unix(),
	}
	p.Tasks <- t
}

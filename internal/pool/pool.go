package pool

import (
	"context"
	"sync"
	"time"

	"github.com/rubiojr/hashub/internal/log"
)

var pool *Pool

type Task struct {
	ID   int64
	Func func() error
}

type Pool struct {
	Tasks       chan Task
	NumWorkers  int
	WorkerGroup sync.WaitGroup
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
		go func(workerID int) {
			defer p.WorkerGroup.Done()
			for {
				select {
				case task := <-p.Tasks:
					if task.ID == 0 {
						return
					}
					if err := task.Func(); err != nil {
						if err == context.Canceled {
							return
						}
						log.Errorf("Worker %d failed to process task %d: %v\n", workerID, task.ID, err)
					}
				}
			}
		}(i)
	}
}

func (p *Pool) Stop() {
	close(p.Tasks)
	p.WorkerGroup.Wait()
}

func (p *Pool) Submit(f func() error) {
	t := Task{
		Func: f,
		ID:   time.Now().Unix(),
	}
	p.Tasks <- t
}

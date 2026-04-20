package main

import (
	"fmt"
	"sync"
	"time"
)

type Job struct {
	id int
}

type Result struct {
	workerID int
	jobID    int
}

func worker(id int, jobCh <-chan Job, resultCh chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()
	for job := range jobCh {
		// 模拟任务耗时
		time.Sleep(300 * time.Millisecond)
		fmt.Printf("worker %d processing job %d\n", id, job.id)
		resultCh <- Result{workerID: id, jobID: job.id}
	}
}

func main() {
	const workerNum = 3
	const jobNum = 10

	jobCh := make(chan Job, jobNum)
	resultCh := make(chan Result, jobNum)

	var wg sync.WaitGroup

	// 启动 worker 池
	for i := 1; i <= workerNum; i++ {
		wg.Add(1)
		go worker(i, jobCh, resultCh, &wg)
	}

	// 提交所有任务
	for i := 1; i <= jobNum; i++ {
		jobCh <- Job{id: i}
	}
	close(jobCh) // 所有任务提交完毕，关闭任务队列

	// 等所有 worker 完成后关闭结果队列
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// 收集所有结果
	for result := range resultCh {
		fmt.Printf("result: worker %d finished job %d\n", result.workerID, result.jobID)
	}

	fmt.Println("all jobs done")
}

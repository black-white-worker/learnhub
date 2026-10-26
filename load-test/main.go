package main

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var token = "2df7e2e8ac3dd4125d593354902a1048dcf38530247810667ae5357079f0cbd0"

type target struct {
	name string
	url  string
}

var targets = []target{
	{"课程列表(缓存)", "http://localhost:8888/api/course/list?page=1&pageSize=20"},
	{"公告列表(并发已读)", "http://localhost:8888/api/class/2/announcements?page=1&pageSize=20"},
	{"考试列表(分页)", "http://localhost:8888/api/class/2/exams?page=1&pageSize=20"},
	{"作业列表(分页)", "http://localhost:8888/api/class/2/assignments?page=1&pageSize=20"},
}

func main() {
	fmt.Println("=== 学习平台 本机压测 ===")
	fmt.Printf("压测参数: 每个接口 50 并发, 持续 5 秒\n\n")

	for _, t := range targets {
		fmt.Printf("--- 压测: %s ---\n", t.name)
		runBench(t.url, 50, 5*time.Second)
		fmt.Println()
	}
}

func runBench(url string, concurrency int, duration time.Duration) {
	var wg sync.WaitGroup
	var success int64
	var fail int64
	var latencies []time.Duration
	var mu sync.Mutex

	// 复用 transport，避免每次请求新建连接
	transport := &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 200,
		MaxConnsPerHost:     200,
		IdleConnTimeout:     30 * time.Second,
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: transport}
	deadline := time.Now().Add(duration)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				req, _ := http.NewRequest("GET", url, nil)
				req.Header.Set("Authorization", "Bearer "+token)
				start := time.Now()
				resp, err := client.Do(req)
				lat := time.Since(start)
				if err != nil {
					atomic.AddInt64(&fail, 1)
					continue
				}
				resp.Body.Close()
				if resp.StatusCode == 200 {
					atomic.AddInt64(&success, 1)
					mu.Lock()
					latencies = append(latencies, lat)
					mu.Unlock()
				} else {
					atomic.AddInt64(&fail, 1)
				}
			}
		}()
	}
	wg.Wait()

	total := success + fail
	qps := float64(success) / duration.Seconds()

	mu.Lock()
	defer mu.Unlock()
	if len(latencies) == 0 {
		fmt.Println("  无成功请求")
		return
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[len(latencies)*50/100]
	p95 := latencies[len(latencies)*95/100]
	p99 := latencies[len(latencies)*99/100]

	fmt.Printf("  总请求: %d, 成功: %d, 失败: %d\n", total, success, fail)
	fmt.Printf("  QPS: %.1f\n", qps)
	fmt.Printf("  P50: %v, P95: %v, P99: %v\n", p50.Round(time.Microsecond), p95.Round(time.Microsecond), p99.Round(time.Microsecond))
	fmt.Printf("  错误率: %.2f%%\n", float64(fail)/float64(total)*100)
}

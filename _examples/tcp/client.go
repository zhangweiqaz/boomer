package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/zhangweiqaz/boomer"
	"log"
	"net"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

// 服务端的链接地址
var bindHost string
var bindPort string

// 用于master下发停止压测事件时，退出所有的客户端协程
var stopChannel chan bool

// 客户端id，用于唯一标记一个客户端
// 正常测试中，客户端id范围，可以通过参数传入，确保每个worker id范围不一样
// 或者写个函数生成id，保证每个协程的客户端id不一样
var playerIdStart int64 = 0

// 客户端行为逻辑函数
// 这里模拟开启tcp链接，并发送2种信息
// 压测过程中，每个客户端协程独立执行该函数，如果函数执行后退出或异常，协程会循环执行该函数，直到压测停止，或进程退出
func worker1(ctx context.Context) {
	// 简单模拟客户端id自增，实际情况中根据业务要求改写
	myPlayId := atomic.AddInt64(&playerIdStart, 1)
	log.Println("*******player:", myPlayId, "created")
	// 链接服务端
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", bindHost, bindPort))
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()

	readBuff := make([]byte, 5)

	// 循环执行压测请求逻辑
	// 这里只是演示两种请求
	// 实际这里可能设计一些随机逻辑，或行为树等，模拟真实客户端行为
	for {
		select {
		// 压测任务停止
		case <-stopChannel:
			log.Println("myPlayId：", myPlayId, "获取到 stop信号，停止")
			playerIdStart = 0
			return
		// 进程终止
		case <-ctx.Done():
			log.Println("myPlayId：", myPlayId, "获取到进程停止信号,停止")
			return
		// 具体的客户端逻辑
		default:
			// 第一种请求
			start := time.Now()
			conn.SetWriteDeadline(time.Now().Add(time.Second))
			n, err := conn.Write([]byte("hello"))
			elapsed := time.Since(start)
			if err != nil {
				// 上报错误
				boomer.RecordFailure("tcp11", "write failure", elapsed.Nanoseconds()/int64(time.Millisecond), err.Error())
				continue
			}
			// len("hello") == 5
			if n != 5 {
				// 上报错误
				boomer.RecordFailure("tcp11", "write mismatch", elapsed.Nanoseconds()/int64(time.Millisecond), "write mismatch")
				continue
			}

			conn.SetReadDeadline(time.Now().Add(time.Second))
			n, err = conn.Read(readBuff)
			time.Sleep(time.Millisecond * 2000)
			elapsed = time.Since(start)
			if err != nil {
				// 上报错误
				boomer.RecordFailure("tcp11", "read failure", elapsed.Nanoseconds()/int64(time.Millisecond), err.Error())
				continue
			}

			if n != 5 {
				// 上报错误
				boomer.RecordFailure("tcp11", "read mismatch", elapsed.Nanoseconds()/int64(time.Millisecond), "read mismatch")
				continue
			}
			// 上报成功
			boomer.RecordSuccess("tcp11", "success", elapsed.Nanoseconds()/int64(time.Millisecond), 5)

			// 第二种请求
			start = time.Now()
			conn.SetWriteDeadline(time.Now().Add(time.Second))
			n, err = conn.Write([]byte("world"))
			elapsed = time.Since(start)
			if err != nil {
				// 上报错误
				boomer.RecordFailure("tcp12", "write failure", elapsed.Nanoseconds()/int64(time.Millisecond), err.Error())
				continue
			}
			// len("hello") == 5
			if n != 5 {
				// 上报错误
				boomer.RecordFailure("tcp12", "write mismatch", elapsed.Nanoseconds()/int64(time.Millisecond), "write mismatch")
				continue
			}

			conn.SetReadDeadline(time.Now().Add(time.Second))
			n, err = conn.Read(readBuff)
			time.Sleep(time.Millisecond * 2000)
			elapsed = time.Since(start)
			if err != nil {
				// 上报错误
				boomer.RecordFailure("tcp12", "read failure", elapsed.Nanoseconds()/int64(time.Millisecond), err.Error())
				continue
			}

			if n != 5 {
				// 上报错误
				boomer.RecordFailure("tcp12", "read mismatch", elapsed.Nanoseconds()/int64(time.Millisecond), "read mismatch")
				continue
			}

			boomer.RecordSuccess("tcp12", "success", elapsed.Nanoseconds()/int64(time.Millisecond), 5)
		}
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	flag.Parse()
	// 将函数转成task
	// 可以有多个task，来借用boomer自己的weight来进行task随机，
	// 也可以只有一个task，自己在函数里面实现随机
	task1 := &boomer.Task{
		Name:   "tcp1",
		Weight: 10,
		Fn:     worker1,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-ctx.Done()
		time.Sleep(time.Second * 10)
		stop()
	}()

	boomer.Events.Subscribe(boomer.EVENT_SPAWN, func(workers int, spawnRate float64) {
		stopChannel = make(chan bool)
	})

	boomer.Events.Subscribe(boomer.EVENT_STOP, func() {
		log.Println("main 获取到 stop信号")
		close(stopChannel)
	})

	boomer.Run(ctx, task1)
}

func init() {
	flag.StringVar(&bindHost, "host", "127.0.0.1", "host")
	flag.StringVar(&bindPort, "port", "4567", "port")
	flag.Int64Var(&playerIdStart, "playerIdStart", 0, "playerIdStart")
}

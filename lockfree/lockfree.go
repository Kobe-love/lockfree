/*
 * Copyright (C) THL A29 Limited, a Tencent company. All rights reserved.
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package lockfree

import (
	"fmt"
	"sync/atomic"
)

// Lockfree 包装类，内部包装了生产者和消费者
type Lockfree[T any] struct {
	writer   *Producer[T]
	consumer *consumer[T]
	status   int32
}

// NewLockfree 自定义创建消费端的Disruptor
// parallel：表示是否并行化处理
// capacity：buffer的容量大小，类似于chan的大小，但要求必须是2^n，即2的指数倍
// handler：消费端的事件处理器
// writeWait：写入阻塞时等待策略，建议使用SchedWaitStrategy
func NewLockfree[T any](capacity int, handler EventHandler[T], blocks blockStrategy) *Lockfree[T] {
	seqer := newSequencer(capacity)
	abuf := newAvailable(capacity)
	rbuf := newRingBuffer[T](capacity, seqer)
	cmer := newConsumer[T](rbuf, abuf, handler, blocks)
	writer := newProducer[T](seqer, abuf, rbuf, blocks)
	return &Lockfree[T]{
		writer:   writer,
		consumer: cmer,
		status:   READY,
	}
}

func (d *Lockfree[T]) Start() error {
	if atomic.CompareAndSwapInt32(&d.status, READY, RUNNING) {
		// 启动消费者
		if err := d.consumer.start(); err != nil {
			// 恢复现场
			atomic.CompareAndSwapInt32(&d.status, RUNNING, READY)
			return err
		}
		// 启动生产者
		if err := d.writer.start(); err != nil {
			// 恢复现场
			atomic.CompareAndSwapInt32(&d.status, RUNNING, READY)
			return err
		}
		return nil
	}
	return fmt.Errorf(StartErrorFormat, "Disruptor")
}

func (d *Lockfree[T]) Producer() *Producer[T] {
	return d.writer
}

func (d *Lockfree[T]) Running() bool {
	return d.status == RUNNING
}

func (d *Lockfree[T]) Close() error {
	if atomic.CompareAndSwapInt32(&d.status, RUNNING, READY) {
		// 关闭生产者
		if err := d.writer.close(); err != nil {
			// 恢复现场
			atomic.CompareAndSwapInt32(&d.status, READY, RUNNING)
			return err
		}
		// 关闭消费者
		if err := d.consumer.close(); err != nil {
			// 恢复现场
			atomic.CompareAndSwapInt32(&d.status, READY, RUNNING)
			return err
		}
		// 关闭成功
		return nil
	}
	return fmt.Errorf(CloseErrorFormat, "Disruptor")
}
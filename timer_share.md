---
theme: .theme.json
---

# 目录

* timer的历史
* 简单使用
  * 注意的坑
* 源码分析
  * timer的数据结构
  * timer的状态变更
  * timer的触发时机
* Q&A

---

# Timer的历史

1. Go 1.9 版本之前，所有的计时器由全局唯一的四叉堆维护；
2. Go 1.10~1.13，全局使用64个四叉堆维护全部的计时器，每个处理器（P）创建的计时器会由对应的四叉堆维护；
3. Go 1.14 版本之后，每个处理器单独管理计时器并通过网络轮询器触发；

---

## 全局四叉堆（1.10之前）

```go
var timers struct {
	lock         mutex
	gp           *g
	created      bool
	sleeping     bool
	rescheduling bool
	sleepUntil   int64
	waitnote     note
	t            []*timer
}
```



                             ┌─────────┐                         
                             │  timer  │                         
                             └─────────┘                         
                                  ▲                              
            ┌──────────────┬──────┴─────────┬──────────────┐     
            │              │                │              │     
       ┌─────────┐    ┌─────────┐      ┌─────────┐    ┌─────────┐
       │  timer  │    │  timer  │      │  timer  │    │  timer  │
       └─────────┘    └─────────┘      └─────────┘    └─────────┘
            ▲                                                    
     ┌──────┴─────┐                                              
     │            │                                              
┌─────────┐  ┌─────────┐                                         
│  timer  │  │  timer  │                                         
└─────────┘  └─────────┘    


全局四叉堆共用的互斥锁对计时器的性能影响非常大

---

## 分片四叉堆（1.10~1.13）

```go
const timersLen = 64

var timers [timersLen]struct {
	timersBucket
}

type timersBucket struct {
	lock         mutex
	gp           *g
	created      bool
	sleeping     bool
	rescheduling bool
	sleepUntil   int64
	waitnote     note
	t            []*timer
}
```


                             ┌────────┬────────┬────────┐        ┌────────┐
                             │ bucket │ bucket │ bucket │  ...   │ bucket │
                             └────────┴────────┴────────┘        └────────┘
                                  ▲                                        
                                  │                                        
                                  │                                        
                             ┌─────────┐                                   
                             │  timer  │                                   
                             └─────────┘                                   
                                  ▲                                        
            ┌──────────────┬──────┴─────────┬──────────────┐               
            │              │                │              │               
       ┌─────────┐    ┌─────────┐      ┌─────────┐    ┌─────────┐          
       │  timer  │    │  timer  │      │  timer  │    │  timer  │          
       └─────────┘    └─────────┘      └─────────┘    └─────────┘          
            ▲                                                              
     ┌──────┴─────┐                                                        
     │            │                                                        
┌─────────┐  ┌─────────┐                                                   
│  timer  │  │  timer  │                                                   
└─────────┘  └─────────┘    

---

## 分片四叉堆（1.10~1.13）

在理想情况下，四叉堆的数量应该等于处理器的数量，需要动态分配，最终选择初始化 64 个四叉堆

```go
func timerproc(tb *timersBucket) {
	tb.gp = getg()
	for {
    tb.sleeping = false
    now := nanotime()
    delta := int64(-1)
    for {
      if len(tb.t) == 0 {
        delta = -1
        break
      }
      t := tb.t[0]
      delta = t.when - now
      if delta > 0 { // 还没到时间
        break
      }

      // 执行timer，并从堆上移除timer
      last := len(tb.t) - 1
      if last > 0 {
        tb.t[0] = tb.t[last]
        tb.t[0].i = 0
      }
      tb.t[last] = nil
      tb.t = tb.t[:last]
      ...
      f(arg, seq)
    }
    ...
    notetsleepg(&tb.waitnote, delta)
	}
}
```

### 问题

* 造成的处理器和线程之间频繁的上下文切换却成为了影响计时器性能的首要因素(Question1: why?)

---

## P单独计数器（1.14之后）

```go
type p struct {
	...
	timersLock mutex // 保护计时器的互斥锁
	timers []*timer // 四叉堆

	numTimers     uint32 // 处理器中的计时器数量
	adjustTimers  uint32 // 处理器中处于 timerModifiedEarlier 状态的计时器数量
	deletedTimers uint32 // 处理器中处于 timerDeleted 状态的计时器数量；
	...
}
```

                             ┌────────┬────────┬────────┐        ┌────────┐
                             │   p    │   p    │   p    │  ...   │   p    │
                             └────────┴────────┴────────┘        └────────┘
                                  ▲                                        
                                  │                                        
                                  │                                        
                             ┌─────────┐                                   
                             │  timer  │                                   
                             └─────────┘                                   
                                  ▲                                        
            ┌──────────────┬──────┴─────────┬──────────────┐               
            │              │                │              │               
       ┌─────────┐    ┌─────────┐      ┌─────────┐    ┌─────────┐          
       │  timer  │    │  timer  │      │  timer  │    │  timer  │          
       └─────────┘    └─────────┘      └─────────┘    └─────────┘          
            ▲                                                              
     ┌──────┴─────┐                                                        
     │            │                                                        
┌─────────┐  ┌─────────┐                                                   
│  timer  │  │  timer  │                                                   
└─────────┘  └─────────┘                                                   

由处理器的网络轮询器和调度器触发

* 充分利用本地性
* 减少上下文的切换开销

---

# 简单使用

主要分为2类，一次性触发的timer和多次触发的ticker

```go
func TestTimer(t *testing.T) {
	timer := time.NewTimer(time.Second)

	var ch chan int
	for {
    select {
    case tm := <-timer.C:
      t.Log(tm)
      timer.Reset(time.Second)
    case <-ch:
    }
	}
}
```

```go
func TestTicker(t *testing.T) {
	ticker := time.NewTicker(time.Second)
	var ch chan int
	for {
    select {
    case tm := <-ticker.C:
      t.Log(tm)
    case <-ch:
    }
	}
}
```

除了直接使用以外

* context.WithTimeout
* ...

---

## 使用中的坑

```go
func main() {
  ch := make(chan int, 10)
  go func() {
    in := 1
    for {
        in++
        ch <- in
    }
  }()
  
  for {
    select {
    case _ = <-ch:
      // do something...
      continue
    case <-time.After(3 * time.Minute):
      fmt.Printf("现在是：%d，我脑子进煎鱼了！", time.Now().Unix())
    }
  }
}
```

例子与解析参考: https://mp.weixin.qq.com/s/KSBdPkkvonSES9Z9iggElg

---

# 源码分析

```go
func NewTimer(d Duration) *Timer {
  c := make(chan Time, 1)
  t := &Timer{
    C: c,
    r: runtimeTimer{
      when: when(d),
      f:    sendTime,
      arg:  c,
    },
	}
  startTimer(&t.r)
  return t
}
```

可以看到NewTimer和NewTicker都会初始化runtimeTimer，差别在于Ticker会比Timer多了period参数。最后调用startTimer将timer添加到底层的最小四叉堆中

---

## 数据结构

```go
type timer struct {
	// If this timer is on a heap, which P's heap it is on.
	// puintptr rather than *p to match uintptr in the versions
	// of this struct defined in other packages.
	pp puintptr // 当前P的指针

	// Timer wakes up at when, and then at when+period, ... (period > 0 only)
	// each time calling f(arg, now) in the timer goroutine, so f must be
	// a well-behaved function and not block.
	//
	// when must be positive on an active timer.
	when   int64                      // 当前计时器被唤醒的时间
	period int64                      // 两次被唤醒的间隔
	f      func(interface{}, uintptr) // 每当计时器被唤醒时都会调用的函数
	arg    interface{}                // 计时器被唤醒时调用 f 传入的参数
	seq    uintptr

	// What to set the when field to in timerModifiedXX status.
	nextwhen int64 // 计时器处于 timerModifiedXX 状态时，用于设置 when 字段；

	// The status field holds one of the values below.
	status uint32 // 计时器的状态
}
```

---

## timer的状态变更

timer的操作都是对timer状态的变更和四叉堆的维护

```go
const (
    // 还没有设置状态
    timerNoStatus = iota

    // 等待被调用
    // timer 已在 P 的列表中
    timerWaiting

    // 表示 timer 在运行中 
    timerRunning

    // timer 已被删除 
    timerDeleted

    // timer 正在被移除 
    timerRemoving

    // timer 已被移除，并停止运行 
    timerRemoved

    // timer 被修改了 
    timerModifying

    // 被修改到了更早的时间 
    timerModifiedEarlier 

    // 被修改到了更晚的时间
    timerModifiedLater

    // 已经被修改，并且正在被移动
    timerMoving
)
```

* timerRunning、timerRemoving、timerModifying 和 timerMoving — 停留的时间都比较短；
* timerWaiting、timerRunning、timerDeleted、timerRemoving、timerModifying、timerModifiedEarlier、timerModifiedLater 和 timerMoving — 计时器在处理器的堆上；
* timerNoStatus 和 timerRemoved — 计时器不在堆上；
* timerModifiedEarlier 和 timerModifiedLater — 计时器虽然在堆上，但是可能位于错误的位置上，需要重新排序；

---

### 添加timer

```go
// 把 t 添加到 timer 堆
// startTimer adds t to the timer heap.
//go:linkname startTimer time.startTimer
func startTimer(t *timer) {
	if raceenabled {
    racerelease(unsafe.Pointer(t))
	}
	addtimer(t)
}
```

继续调用addtimer方法

```go
// addtimer adds a timer to the current P.
// This should only be called with a newly created timer.
// That avoids the risk of changing the when field of a timer in some P's heap,
// which could cause the heap to become unsorted.
func addtimer(t *timer) {
	// when must be positive. A negative value will cause runtimer to
	// overflow during its delta calculation and never expire other runtime
	// timers. Zero will cause checkTimers to fail to notice the timer.
	if t.when <= 0 {
		throw("timer when must be positive")
	}
	if t.period < 0 {
		throw("timer period must be non-negative")
	}
	if t.status != timerNoStatus { // 添加新的timer必须是timerNoStatus
		throw("addtimer called with initialized timer")
	}
	t.status = timerWaiting

	when := t.when

	pp := getg().m.p.ptr()
	lock(&pp.timersLock)
	cleantimers(pp)
	doaddtimer(pp, t)
	unlock(&pp.timersLock)

	wakeNetPoller(when)
}
```

---

```go
// 注意cleantimers清理的是堆顶部的timer，只要顶部是timerDeleted，timerModifiedEarlier/timerModifiedLater的timer都会处理
// 处理完后会调整堆，再处理堆顶部的timer，所以不只是处理1个timer，
// cleantimers会出现下面2种状态的变化，也就是清除已经删除的，移动timer0
// timerDeleted -> timerRemoving -> timerRemoved
// timerModifiedEarlier/timerModifiedLater -> timerMoving -> timerWaiting
func cleantimers(pp *p) {
  gp := getg()
  for {
    if len(pp.timers) == 0 {
      return
    }

    t := pp.timers[0] // 堆顶，when最小，最早发生的timer

    switch s := atomic.Load(&t.status); s {
    case timerDeleted:
      // timerDeleted --> timerRemoving --> 从堆中删除timer --> timerRemoved
      if !atomic.Cas(&t.status, s, timerRemoving) {
        continue
      }
      dodeltimer0(pp)
      if !atomic.Cas(&t.status, timerRemoving, timerRemoved) {
        badTimer()
      }
      atomic.Xadd(&pp.deletedTimers, -1)
    case timerModifiedEarlier, timerModifiedLater: // TODO 如果modTimer将非timer0的when改成了比timer0更先触发的时候是怎么处理的
      // timerMoving --> 调整 timer 的时间 --> timerWaiting
      // 此时 timer 被调整为更早或更晚，将原先的 timer 进行删除，再重新添加
      if !atomic.Cas(&t.status, s, timerMoving) {
        continue
      }
      // Now we can change the when field.
      t.when = t.nextwhen
      // Move t to the right position.
      // 删除原来的
      dodeltimer0(pp)
      // 然后再重新添加
      doaddtimer(pp, t)
      if s == timerModifiedEarlier {
        atomic.Xadd(&pp.adjustTimers, -1) // 如果t0之前是timerModifiedEarlier，因为已经调整了t0，所以需要将adjustTimers减1
      }
      if !atomic.Cas(&t.status, timerMoving, timerWaiting) {
        badTimer()
      }
    default:
      return
    }
  }
}
```

---

```go
// doaddtimer adds t to the current P's heap.
// The caller must have locked the timers for pp.
func doaddtimer(pp *p, t *timer) {
  // Timers依赖network poller，确保netpoll经初始化了
  // Timers rely on the network poller, so make sure the poller
  // has started.
  if netpollInited == 0 {
    netpollGenericInit()
  }

  if t.pp != 0 { // 创建timer时没有绑定p，如果p存在的话属于异常情况
    throw("doaddtimer: P already set in timer")
  }
  t.pp.set(pp) // timer绑定到当前P的堆上
  i := len(pp.timers)
  pp.timers = append(pp.timers, t)
  siftupTimer(pp.timers, i) // 调整4叉堆
  if t == pp.timers[0] {    // 如果新加入的timer是当前p中最新触发的，将t.when保存到pp.timer0When
    atomic.Store64(&pp.timer0When, uint64(t.when))
  }
  atomic.Xadd(&pp.numTimers, 1)
}
```

---

doaddtimer方法返回后，回到addtimer方法会调用wakeNetPoller方法

```go
// wakeNetPoller wakes up the thread sleeping in the network poller if it isn't
// going to wake up before the when argument; or it wakes an idle P to service
// timers and the network poller if there isn't one already.
func wakeNetPoller(when int64) {
  if atomic.Load64(&sched.lastpoll) == 0 {
    // In findrunnable we ensure that when polling the pollUntil
    // field is either zero or the time to which the current
    // poll is expected to run. This can have a spurious wakeup
    // but should never miss a wakeup.
    pollerPollUntil := int64(atomic.Load64(&sched.pollUntil))
    if pollerPollUntil == 0 || pollerPollUntil > when { // 网络轮询器poll > timer的触发时间，立即唤醒netpoll
      netpollBreak()
    }
  } else {
    if GOOS != "plan9" { // Temporary workaround - see issue #42303.
      wakep()
    }
  }
}
```

```go
// netpollBreak interrupts a kevent.
func netpollBreak() {
  if atomic.Cas(&netpollWakeSig, 0, 1) {
    for {
      var b byte
      n := write(netpollBreakWr, unsafe.Pointer(&b), 1)
      if n == 1 || n == -_EAGAIN {
        break
      }
      if n == -_EINTR {
        continue
      }
      println("runtime: netpollBreak write failed with", -n)
      throw("runtime: netpollBreak write failed")
    }
  }
}
```

---

```go
// netpoll checks for ready network connections.
// Returns list of goroutines that become runnable.
// delay < 0: blocks indefinitely
// delay == 0: does not block, just polls
// delay > 0: block for up to that many nanoseconds
// delay < 0 无限block等待
// delay == 0 不会block
// delay block 最多delay时间
// runtime.netpoll 返回的 Goroutine 列表都会被 runtime.injectglist 注入到处理器或者全局的运行队列上。
// 因为系统监控 Goroutine 直接运行在线程上，所以它获取的 Goroutine 列表会直接加入全局的运行队列，
// 其他 Goroutine 获取的列表都会加入 Goroutine 所在处理器的运行队列上。
func netpoll(delay int64) gList {
    var events [128]epollevent
  retry:
    // 等待文件描述符转换成可读或者可写
    n := epollwait(epfd, &events[0], int32(len(events)), waitms)
    // 当 epollwait 系统调用返回的值大于 0 时，意味着被监控的文件描述符出现了待处理的事件
    var toRun gList
    for i := int32(0); i < n; i++ {
        ev := &events[i]
        if ev.events == 0 {
            continue
        }

        // runtime.netpollBreak 触发的事件
        if *(**uintptr)(unsafe.Pointer(&ev.data)) == &netpollBreakRd {
            if ev.events != _EPOLLIN {
                println("runtime: netpoll: break fd ready for", ev.events)
                throw("runtime: netpoll: break fd ready for something unexpected")
            }
            if delay != 0 {
                // netpollBreak could be picked up by a
                // nonblocking poll. Only read the byte
                // if blocking.
                var tmp [16]byte
                read(int32(netpollBreakRd), noescape(unsafe.Pointer(&tmp[0])), int32(len(tmp)))
                atomic.Store(&netpollWakeSig, 0)
            }
            continue
        }

        // 另一种是其他文件描述符的正常读写事件
    }
    return toRun
}
```

### 问题

netpoll对netpollBreakRd的事件处理逻辑里面并没有对timer的操作，netpoll对timer的作用是啥？

---

### 停止timer

```go
func (t *Timer) Stop() bool {
  if t.r.f == nil {
    panic("time: Stop called on uninitialized Timer")
  }
  return stopTimer(&t.r)
}
```

```go
func (t *Ticker) Stop() {
  stopTimer(&t.r)
}
```

```go
func stopTimer(t *timer) bool {
  return deltimer(t)
}
```

---


```go
// 返回的是这个timer在执行前被移除的，已经执行过了就返回false，还没有执行就返回true
func deltimer(t *timer) bool {
  for {
    switch s := atomic.Load(&t.status); s {
    case timerWaiting, timerModifiedLater: // timer还没启动或修改为更晚的时间
      mp := acquirem()
      // timerWaiting/timerModifiedLater --> timerModifying --> timerDeleted
      if atomic.Cas(&t.status, s, timerModifying) { // TODO 为什么要先切换为timerModifying
        tpp := t.pp.ptr()
        if !atomic.Cas(&t.status, timerModifying, timerDeleted) { // 置为timerDeleted状态
          badTimer()
        }
        releasem(mp)
        atomic.Xadd(&tpp.deletedTimers, 1)
        // Timer was not yet run.
        return true
      } else { // 修改为timerModifying失败，说明t的状态已经不再是timerWaiting, timerModifiedLater了
        releasem(mp) // 下一次再来处理
      }
    case timerModifiedEarlier:
      mp := acquirem()
      // timerModifiedEarlier --> timerModifying --> timerDeleted
      if atomic.Cas(&t.status, s, timerModifying) {
        tpp := t.pp.ptr()
        atomic.Xadd(&tpp.adjustTimers, -1) // timerModifiedEarlier的timer被stop了，所以需要将adjustTimers-1
        if !atomic.Cas(&t.status, timerModifying, timerDeleted) {
          badTimer()
        }
        releasem(mp)
        atomic.Xadd(&tpp.deletedTimers, 1)
        // Timer was not yet run.
        return true
      } else {
        releasem(mp) // 下一次再来处理
      }
    case timerDeleted, timerRemoving, timerRemoved:
      // Timer 已经运行
      return false
    case timerRunning, timerMoving:
      // 正在执行或被移动了，等待完成，下一次再来处理
      osyield()
    case timerNoStatus:
      // Removing timer that was never added or
      // has already been run. Also see issue 21874.
      return false
    case timerModifying:
      // 同时调用了deltimer，modtimer；等待其他调用完成，下一次再来处理
      osyield()
    default:
      badTimer()
    }
  }
}
```

---

### 问题

1. 为什么timer状态变化的时候需要需要先改为timerModifying然后再修改成最后的状态？

在timer的status状态常量这有这么一段注释

```
// We don't permit calling addtimer/deltimer/modtimer/resettimer simultaneously,
// but adjusttimers and runtimer can be called at the same time as any of those.
```

为了保证addtimer/deltimer/modtimer/resettimer不能被同时调用，所以需要timerModifying这个状态

2. deltimer并没有从 四叉堆中删除timer，只是将timer的状态切换成timerDeleted，这个是为什么？

这个在deltimer的注释上已经说明了

```
// deltimer deletes the timer t. It may be on some other P, so we can't
// actually remove it from the timers heap. We can only mark it as deleted.
// It will be removed in due course by the P whose heap it is on.
```

deltimer删除的timer可能在其他P上，以为调度循环的 时候不仅会从其他P上偷G，还会偷timer，所以只是对timer进行标记，在timer所在的P中，通过 cleantimers/adjusttimers等方法来真正从堆中删除

---

### 其他timer的方法

* 修改timer => modtimer
* 调整timer => adjusttimers
* 运行timer => runtimer

可参考: https://zengqiang96.github.io/posts/timer%E6%BA%90%E7%A0%81%E9%98%85%E8%AF%BB/

---

# 触发timer

```go
// checkTimers runs any timers for the P that are ready.
// If now is not 0 it is the current time.
// It returns the current time or 0 if it is not known,
// and the time when the next timer should run or 0 if there is no next timer,
// and reports whether it ran any timers.
// If the time when the next timer should run is not 0,
// it is always larger than the returned time.
// We pass now in and out to avoid extra calls of nanotime.
//go:yeswritebarrierrec
func checkTimers(pp *p, now int64) (rnow, pollUntil int64, ran bool) {
  if next == 0 { // 没有timer需要执行和调整
    // No timers to run or adjust.
    return now, 0, false
  }

  if now == 0 {
    now = nanotime()
  }
  if now < next { // 最快的 timer还没到 执行的时间
    if pp != getg().m.p.ptr() || int(atomic.Load(&pp.deletedTimers)) <= int(atomic.Load(&pp.numTimers)/4) {
      return now, next, false
    }
  }

  lock(&pp.timersLock)

  if len(pp.timers) > 0 {
    adjusttimers(pp, now)    // 删除已经执行的timer，调整timerModifiedEarlier 和 timerModifiedLater 的计时器的时间
    for len(pp.timers) > 0 { // 执行所有到期的timer
      if tw := runtimer(pp, now); tw != 0 {
        if tw > 0 {
          pollUntil = tw
        }
        break
      }
      ran = true
    }
  }

  // 当前 Goroutine 的处理器和传入的处理器相同,并且处理器中删除的计时器是堆中计时器的 1/4 以上，
  if pp == getg().m.p.ptr() && int(atomic.Load(&pp.deletedTimers)) > len(pp.timers)/4 {
    clearDeletedTimers(pp)
  }

  unlock(&pp.timersLock)

  return now, pollUntil, ran
}
```

---

# 触发timer

runtime.checkTimers 是调度器用来运行处理器中计时器的函数，它会在发生以下情况时被调用：

* 调度器调用 runtime.schedule 执行调度时；
* 调度器调用 runtime.findrunnable 获取可执行的 Goroutine 时；
* 调度器调用 runtime.findrunnable 从其他处理器窃取计时器时；
* 系统监控函数 runtime.sysmon

---


# Q&A

1. 造成的处理器和线程之间频繁的上下文切换却成为了影响计时器性能的首要因素(why)?
2. 为什么timer状态变化的时候需要需要先改为timerModifying然后再修改成最后的状态？
3. deltimer并没有从 四叉堆中删除timer，只是将timer的状态切换成timerDeleted，这个是为什么？
4. netpoll对netpollBreakRd的事件处理逻辑里面并没有对timer的操作，netpoll对timer的作用是啥？





package cron

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Cron会跟踪任意数量的条目，并按计划指定的方式调用关联的func。
// 它可以启动，停止，并且可以在运行时检查条目。
type Cron struct {
	entries   []*Entry
	chain     Chain
	stop      chan struct{}
	add       chan *Entry
	remove    chan EntryID
	snapshot  chan chan []Entry
	running   bool
	logger    Logger
	runningMu sync.Mutex
	location  *time.Location
	parser    ScheduleParser
	nextID    EntryID
	jobWaiter sync.WaitGroup
}

// ScheduleParser是一个接口，用于将调度的spec参数转化为Schedule对象
type ScheduleParser interface {
	Parse(spec string) (Schedule, error)
}

// Job 是一个接口，用户提交cron任务的
type Job interface {
	Run()
}

// Schedule描述了一个Job的工作周期
type Schedule interface {
	// Next返回下一个激活时间，晚于给定时间。
	// 首先调用Next，然后每次运行作业时调用。
	Next(time.Time) time.Time
}

// EntryID 在一个Cron实例中指定一个条目
type EntryID int

// Entry 包含一个schedule时间表，和在该时间表上执行的函数
type Entry struct {
	// ID是该项目的cron分配的ID，可用于查找快照或将其删除。
	ID EntryID

	// 运行job的Schedule
	Schedule Schedule

	// 下次将运行该作业的时间，或者如果尚未启动Cron或该条目的计划无法满足，则为时间的零值
	Next time.Time

	// Prev是上一次运行此作业的时间，否则为零。
	Prev time.Time
	
	// WrappedJob是激活Schedule后要运行的东西。
	WrappedJob Job

	// Job是提交给cron的东西。
	// 它被保留下来，以便以后需要使用的用户代码，
	// 例如：通过Entries（）可以做到。
	Job Job
}

// 如果不是一个零值的条目, Valid 返回true
func (e Entry) Valid() bool { return e.ID != 0 }

// byTime是一个根据时间排序后的条目（在最后是一个零值的时间）
type byTime []*Entry

func (s byTime) Len() int      { return len(s) }
func (s byTime) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s byTime) Less(i, j int) bool {
	// 两个零值的时间应该返回false.
	// 否则，零值时间比其他任何的时间值都大
	// (在列表最后排序它)
	if s[i].Next.IsZero() {
		return false
	}
	if s[j].Next.IsZero() {
		return true
	}
	return s[i].Next.Before(s[j].Next)
}

// New返回一个新的Cron作业运行程序，并通过给定的选项进行了修改。
//
// 可用的设置
//
//   时区
//     描述:       解释时间表的时区
//     默认值:     time.Local
//
//   解析器
//     描述: 解析器将cron spec字符串转换为cron.Schedules。
//     默认值:     可用的spec形式: https://en.wikipedia.org/wiki/Cron
//
//   链
//     描述: 包装提交的作业以自定义行为。
//     默认值:     恢复异常并打印日志到stderr的chain.
//
// 查看 "cron.With*"方法修改默认的行为。
func New(opts ...Option) *Cron {
	c := &Cron{
		entries:   nil,
		chain:     NewChain(),
		add:       make(chan *Entry),
		stop:      make(chan struct{}),
		snapshot:  make(chan chan []Entry),
		remove:    make(chan EntryID),
		running:   false,
		runningMu: sync.Mutex{},
		logger:    DefaultLogger,
		location:  time.Local,
		parser:    standardParser,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// FuncJob 是一个包装器，将一个函数func()变成一个cron.Job
type FuncJob func()

func (f FuncJob) Run() { f() }

// AddFunc增加一个函数到Cron上，在给定的时间表中运行。
// spec使用Cron实例默认的时区进行解析。
// 返回一个不透明的ID，可用于以后将其删除。
func (c *Cron) AddFunc(spec string, cmd func()) (EntryID, error) {
	return c.AddJob(spec, FuncJob(cmd))
}

// AddJob将作业添加到Cron中，以便按给定的时间表运行。
// spec使用Cron实例默认的时区进行解析。
// 返回一个不透明的ID，可用于以后将其删除。
func (c *Cron) AddJob(spec string, cmd Job) (EntryID, error) {
	schedule, err := c.parser.Parse(spec)
	if err != nil {
		return 0, err
	}
	return c.Schedule(schedule, cmd), nil
}

// 将作业添加到Cron中，以便按给定的时间表运行。
// 该作业由配置的Chain包裹。
func (c *Cron) Schedule(schedule Schedule, cmd Job) EntryID {
	c.runningMu.Lock()
	defer c.runningMu.Unlock()
	c.nextID++
	entry := &Entry{
		ID:         c.nextID,
		Schedule:   schedule,
		WrappedJob: c.chain.Then(cmd),
		Job:        cmd,
	}
	if !c.running {
		c.entries = append(c.entries, entry)
	} else {
		c.add <- entry
	}
	return entry.ID
}

// Entries返回cron条目的快照
func (c *Cron) Entries() []Entry {
	c.runningMu.Lock()
	defer c.runningMu.Unlock()
	if c.running {
		replyChan := make(chan []Entry, 1)
		c.snapshot <- replyChan
		return <-replyChan
	}
	return c.entrySnapshot()
}

// Location获取时区
func (c *Cron) Location() *time.Location {
	return c.location
}

// Entry返回给定条目的快照，或者为空，如果未发现的话。
func (c *Cron) Entry(id EntryID) Entry {
	for _, entry := range c.Entries() {
		if id == entry.ID {
			return entry
		}
	}
	return Entry{}
}

// 删除将来运行的条目。
func (c *Cron) Remove(id EntryID) {
	c.runningMu.Lock()
	defer c.runningMu.Unlock()
	if c.running {
		c.remove <- id
	} else {
		c.removeEntry(id)
	}
}

// 在它自己的协程中启动cron时间表，或者在已经启动后不做任何操作。
func (c *Cron) Start() {
	c.runningMu.Lock()
	defer c.runningMu.Unlock()
	if c.running {
		return
	}
	c.running = true
	go c.run()
}

// 运行cron调度程序;如果已经运行，不做任何操作。
func (c *Cron) Run() {
	c.runningMu.Lock()
	if c.running {
		c.runningMu.Unlock()
		return
	}
	c.running = true
	c.runningMu.Unlock()
	c.run()
}

// 运行调度程序..这是私有的，只是因为需要同步访问“运行中”状态变量。
func (c *Cron) run() {
	c.logger.Info("start")

	// Figure out the next activation times for each entry.
	now := c.now()
	for _, entry := range c.entries {
		entry.Next = entry.Schedule.Next(now)
		c.logger.Info("schedule", "now", now, "entry", entry.ID, "next", entry.Next)
	}

	for {
		// Determine the next entry to run.
		sort.Sort(byTime(c.entries))

		var timer *time.Timer
		if len(c.entries) == 0 || c.entries[0].Next.IsZero() {
			// If there are no entries yet, just sleep - it still handles new entries
			// and stop requests.
			timer = time.NewTimer(100000 * time.Hour)
		} else {
			timer = time.NewTimer(c.entries[0].Next.Sub(now))
		}

		for {
			select {
			case now = <-timer.C:
				now = now.In(c.location)
				c.logger.Info("wake", "now", now)

				// Run every entry whose next time was less than now
				for _, e := range c.entries {
					if e.Next.After(now) || e.Next.IsZero() {
						break
					}
					c.startJob(e.WrappedJob)
					e.Prev = e.Next
					e.Next = e.Schedule.Next(now)
					c.logger.Info("run", "now", now, "entry", e.ID, "next", e.Next)
				}

			case newEntry := <-c.add:
				timer.Stop()
				now = c.now()
				newEntry.Next = newEntry.Schedule.Next(now)
				c.entries = append(c.entries, newEntry)
				c.logger.Info("added", "now", now, "entry", newEntry.ID, "next", newEntry.Next)

			case replyChan := <-c.snapshot:
				replyChan <- c.entrySnapshot()
				continue

			case <-c.stop:
				timer.Stop()
				c.logger.Info("stop")
				return

			case id := <-c.remove:
				timer.Stop()
				now = c.now()
				c.removeEntry(id)
				c.logger.Info("removed", "entry", id)
			}

			break
		}
	}
}

// startJob在新的goroutine中运行给定的作业。
func (c *Cron) startJob(j Job) {
	c.jobWaiter.Add(1)
	go func() {
		defer c.jobWaiter.Done()
		j.Run()
	}()
}

// 现在返回c位置的当前时间
func (c *Cron) now() time.Time {
	return time.Now().In(c.location)
}

// Stop 停止cron调度程序，如果它在运行的话，否则不做任何操作。
// 返回上下文，以便调用方可以等待正在运行的作业完成。
func (c *Cron) Stop() context.Context {
	c.runningMu.Lock()
	defer c.runningMu.Unlock()
	if c.running {
		c.stop <- struct{}{}
		c.running = false
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		c.jobWaiter.Wait()
		cancel()
	}()
	return ctx
}

// entrySnapshot返回当前cron条目列表的一份拷贝。
func (c *Cron) entrySnapshot() []Entry {
	var entries = make([]Entry, len(c.entries))
	for i, e := range c.entries {
		entries[i] = *e
	}
	return entries
}

func (c *Cron) removeEntry(id EntryID) {
	var entries []*Entry
	for _, e := range c.entries {
		if e.ID != id {
			entries = append(entries, e)
		}
	}
	c.entries = entries
}

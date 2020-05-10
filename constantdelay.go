package cron

import "time"

// ConstantDelaySchedule表示一个简单的循环工作周期，例如“每5分钟”。
// 它不支持比每秒更频繁的作业。
type ConstantDelaySchedule struct {
	Delay time.Duration
}

// 每个函数都会返回一个crontab Schedule，每个持续时间都会激活一次。
// 不支持少于一秒的延迟（最多舍入为1秒）。
// 小于秒的任何字段都将被截断。
func Every(duration time.Duration) ConstantDelaySchedule {
	if duration < time.Second {
		duration = time.Second
	}
	return ConstantDelaySchedule{
		Delay: duration - time.Duration(duration.Nanoseconds())%time.Second,
	}
}

// next返回下一次应运行的时间。
// 此操作将四舍五入，以使下一个激活时间为秒。
func (schedule ConstantDelaySchedule) Next(t time.Time) time.Time {
	return t.Add(schedule.Delay - time.Duration(t.Nanosecond())*time.Nanosecond)
}

package cron

import (
	"time"
)

// Option 表示对Cron默认行为的修改。
type Option func(*Cron)

// WithLcation 覆盖cron实例的时区
func WithLocation(loc *time.Location) Option {
	return func(c *Cron) {
		c.location = loc
	}
}

// WithSeconds覆盖用于解释作业计划的解析器，以将秒字段作为第一个字段。
func WithSeconds() Option {
	return WithParser(NewParser(
		Second | Minute | Hour | Dom | Month | Dow | Descriptor,
	))
}

// WithParser覆盖用于解释作业计划的解析器
func WithParser(p ScheduleParser) Option {
	return func(c *Cron) {
		c.parser = p
	}
}

// WithChain指定作业包装程序以应用于添加到此cron的所有作业。
// 有关提供的包装器，请参阅此软件包中的Chain *函数。
func WithChain(wrappers ...JobWrapper) Option {
	return func(c *Cron) {
		c.chain = NewChain(wrappers...)
	}
}

// WithLogger使用提供的日志。
func WithLogger(logger Logger) Option {
	return func(c *Cron) {
		c.logger = logger
	}
}

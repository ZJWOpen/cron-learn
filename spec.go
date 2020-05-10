package cron

import "time"

// SpecSchedule根据传统的crontab 规范指定工作周期（秒粒度）。
// 它最初进行计算并存储为位集。
type SpecSchedule struct {
	Second, Minute, Hour, Dom, Month, Dow uint64

	// 覆盖此时间表的时区
	Location *time.Location
}

// bounds提供了一系列可接受的值（以及名称到值的映射）
type bounds struct {
	min, max uint
	names    map[string]uint
}

// The bounds for each field.
var (
	seconds = bounds{0, 59, nil}
	minutes = bounds{0, 59, nil}
	hours   = bounds{0, 23, nil}
	dom     = bounds{1, 31, nil}
	months  = bounds{1, 12, map[string]uint{
		"jan": 1,
		"feb": 2,
		"mar": 3,
		"apr": 4,
		"may": 5,
		"jun": 6,
		"jul": 7,
		"aug": 8,
		"sep": 9,
		"oct": 10,
		"nov": 11,
		"dec": 12,
	}}
	dow = bounds{0, 6, map[string]uint{
		"sun": 0,
		"mon": 1,
		"tue": 2,
		"wed": 3,
		"thu": 4,
		"fri": 5,
		"sat": 6,
	}}
)

const (
	// 如果表达式中包括星星，则设置最高位。
	starBit = 1 << 63
)

// Next返回该时间表激活后的下一次时间大于给定时间。
// 如果找不到满足时间表的时间，则返回时间的零值。
func (s *SpecSchedule) Next(t time.Time) time.Time {
	// 一般的做法
	//
	// 对于 Month, Day, Hour, Minute, Second:
	// 检查 时间值是否匹配，如果匹配，继续检查下一个字段，
	// 如果字段不匹配时间表，会增加字段，直到匹配为止。
	// 在增加字段时，回绕将其返回到字段列表的开头（因为有必要重新验证以前的字段值）

	// 如果已指定，则将给定时间转换为时间表的时区。
	// 保存原始时区，以便我们在找到时间后可以将其转换回来。
	// 请注意，未指定时区（time.Local）的时间表被视为所提供时间的本地时间。
	origLocation := t.Location()
	loc := s.Location
	if loc == time.Local {
		loc = t.Location()
	}
	if s.Location != time.Local {
		t = t.In(s.Location)
	}

	// 从最早的可能时间（即将到来的第二个时间）开始。
	t = t.Add(1*time.Second - time.Duration(t.Nanosecond())*time.Nanosecond)

	// 此标志指示字段是否已增加。
	added := false

	// 如果五年内没有找到时间，则返回零。
	yearLimit := t.Year() + 5

WRAP:
	if t.Year() > yearLimit {
		return time.Time{}
	}

	// 找到第一个适用月份。
	// 如果是是一个month,不做任何事。
	for 1<<uint(t.Month())&s.Month == 0 {
		// 如果必须增加一个月，请将其他部分重置为0。
		if !added {
			added = true
			// 否则，将日期设置为开始（因为当前时间无关紧要）
			t = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
		}
		t = t.AddDate(0, 1, 0)

		// Wrapped around.
		if t.Month() == time.January {
			goto WRAP
		}
	}

	// 现在在该月获取日期。
	// 注意：这会导致不存在午夜的夏令时问题。例如：圣保罗的DST可以将午夜11/3转换为凌晨1点。
	for !dayMatches(s, t) {
		if !added {
			added = true
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		}
		t = t.AddDate(0, 0, 1)
		// 请注意，由于DST，时间不再是午夜了。
		// 如果是23，则加上一个小时；如果是1，则减去一个小时。
		if t.Hour() != 0 {
			if t.Hour() > 12 {
				t = t.Add(time.Duration(24-t.Hour()) * time.Hour)
			} else {
				t = t.Add(time.Duration(-t.Hour()) * time.Hour)
			}
		}

		if t.Day() == 1 {
			goto WRAP
		}
	}

	for 1<<uint(t.Hour())&s.Hour == 0 {
		if !added {
			added = true
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, loc)
		}
		t = t.Add(1 * time.Hour)

		if t.Hour() == 0 {
			goto WRAP
		}
	}

	for 1<<uint(t.Minute())&s.Minute == 0 {
		if !added {
			added = true
			t = t.Truncate(time.Minute)
		}
		t = t.Add(1 * time.Minute)

		if t.Minute() == 0 {
			goto WRAP
		}
	}

	for 1<<uint(t.Second())&s.Second == 0 {
		if !added {
			added = true
			t = t.Truncate(time.Second)
		}
		t = t.Add(1 * time.Second)

		if t.Second() == 0 {
			goto WRAP
		}
	}

	return t.In(origLocation)
}

// 如果给定时间满足时间表的day-of-week和day-of-month限制，Matches返回true
func dayMatches(s *SpecSchedule, t time.Time) bool {
	var (
		domMatch bool = 1<<uint(t.Day())&s.Dom > 0
		dowMatch bool = 1<<uint(t.Weekday())&s.Dow > 0
	)
	if s.Dom&starBit > 0 || s.Dow&starBit > 0 {
		return domMatch && dowMatch
	}
	return domMatch || dowMatch
}

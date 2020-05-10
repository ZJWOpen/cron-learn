package cron

import (
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"
)

// DefaultLogger 是Cron默认的日志器，如果未制定日志器的话
var DefaultLogger Logger = PrintfLogger(log.New(os.Stdout, "cron: ", log.LstdFlags))

// 调用者可以使用DiscardLogger丢弃所有日志消息。
var DiscardLogger Logger = PrintfLogger(log.New(ioutil.Discard, "", 0))

// Logger是此软件包中用于记录日志的接口，因此可以插入任何后端。 
// 它是github.com/go-logr/logr接口的子集。
type Logger interface {
	// Info 记录有关cron操作的例行消息。
	Info(msg string, keysAndValues ...interface{})
	// Error 记录错误情况。
	Error(err error, msg string, keysAndValues ...interface{})
}

// PrintfLogger将基于Printf的记录器（例如标准库“ log”）包装到Logger接口的实现中
// 该接口仅记录错误。
func PrintfLogger(l interface{ Printf(string, ...interface{}) }) Logger {
	return printfLogger{l, false}
}

// VerbosePrintfLogger将基于Printf的记录器（例如标准库“ log”）包装到Logger接口的实现中
// 该接口记录所有内容。
func VerbosePrintfLogger(l interface{ Printf(string, ...interface{}) }) Logger {
	return printfLogger{l, true}
}

type printfLogger struct {
	logger  interface{ Printf(string, ...interface{}) }
	logInfo bool
}

func (pl printfLogger) Info(msg string, keysAndValues ...interface{}) {
	if pl.logInfo {
		keysAndValues = formatTimes(keysAndValues)
		pl.logger.Printf(
			formatString(len(keysAndValues)),
			append([]interface{}{msg}, keysAndValues...)...)
	}
}

func (pl printfLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	keysAndValues = formatTimes(keysAndValues)
	pl.logger.Printf(
		formatString(len(keysAndValues)+2),
		append([]interface{}{msg, "error", err}, keysAndValues...)...)
}

// formatString返回类似logfmt的格式字符串，用于键/值的数量。
func formatString(numKeysAndValues int) string {
	var sb strings.Builder
	sb.WriteString("%s")
	if numKeysAndValues > 0 {
		sb.WriteString(", ")
	}
	for i := 0; i < numKeysAndValues/2; i++ {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("%v=%v")
	}
	return sb.String()
}

// formatTimes可以随时格式化，时间值为RFC3339。
func formatTimes(keysAndValues []interface{}) []interface{} {
	var formattedArgs []interface{}
	for _, arg := range keysAndValues {
		if t, ok := arg.(time.Time); ok {
			arg = t.Format(time.RFC3339)
		}
		formattedArgs = append(formattedArgs, arg)
	}
	return formattedArgs
}

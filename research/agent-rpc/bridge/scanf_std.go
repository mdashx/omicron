package bridge

import "fmt"

func fmtSscanfImpl(str string, format string, a ...any) (int, error) {
	return fmt.Sscanf(str, format, a...)
}

package hub

import "runtime"

func defaultCPU() int {
	n := runtime.NumCPU()
	if n < 1 {
		return 1
	}
	return n
}

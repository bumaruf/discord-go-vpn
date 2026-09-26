//go:build !linux

package transport

import "fmt"

func newNetns(string, Reporter) (Transport, error) {
	return nil, fmt.Errorf("o modo netns só existe no Linux")
}

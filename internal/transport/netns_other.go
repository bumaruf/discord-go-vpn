//go:build !linux

package transport

import "fmt"

func newNetns() (Transport, error) {
	return nil, fmt.Errorf("o modo netns só existe no Linux")
}

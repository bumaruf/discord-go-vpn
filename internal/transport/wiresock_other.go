//go:build !windows

package transport

import "fmt"

func newWiresock(string, Reporter) (Transport, error) {
	return nil, fmt.Errorf("o modo wiresock só existe no Windows")
}

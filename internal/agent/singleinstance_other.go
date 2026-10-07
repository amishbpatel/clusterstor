//go:build !windows

package agent

func AcquireSingleInstance() (func(),error) {
	return func(){},nil
}

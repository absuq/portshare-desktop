//go:build !windows

package localhostbridge

func NewScanner() Scanner {
	return emptyScanner{}
}

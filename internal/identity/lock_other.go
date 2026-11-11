//go:build !unix

package identity

import "os"

// flock takes no lock where there's no flock: two changes to the accounts
// record at once may lose one.
func flock(*os.File) error {
	return nil
}

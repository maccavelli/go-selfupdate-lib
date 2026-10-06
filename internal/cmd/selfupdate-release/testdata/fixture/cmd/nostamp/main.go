// Command nostamp imports no buildinfo, so a release stamp would be lost:
// selfupdate-release build must refuse it.
package main

import "fmt"

func main() {
	fmt.Println("nostamp")
}

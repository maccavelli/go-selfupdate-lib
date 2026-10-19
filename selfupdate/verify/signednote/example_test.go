package signednote_test

import (
	"fmt"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/verify/signednote"
)

// releaseKeys are the verifier keys the program trusts, embedded as Go
// constants: a CI key and an offline backup key. These are placeholders,
// not keys anyone holds; selfupdate-note keygen makes real ones.
const (
	ciKey     = "github.com/owner/demo/selfupdate+00000000+AX0000000000000000000000000000000000000000000"
	backupKey = "github.com/owner/demo/selfupdate-backup+00000000+AX0000000000000000000000000000000000000000000"
)

// The verifier goes in Config.ManifestVerifiers, beside the rest of the
// program's composition. RequiredFrom is the first signed release: an
// exact older version can still be installed without a note.
func ExampleNew() {
	v, err := signednote.New(signednote.Config{
		Repository:   "github.com/owner/demo",
		Keys:         []string{ciKey, backupKey},
		RequiredFrom: "v1.4.0",
	})
	if err != nil {
		fmt.Println("signednote:", err)
		return
	}
	_ = selfupdate.Config{ManifestVerifiers: []selfupdate.ManifestVerifier{v}}
}

package signednote_test

import (
	"fmt"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/verify/signednote"
)

// ciKey and backupKey are the verifier keys the program trusts, embedded
// as Go constants: a CI key and an offline backup key. They are real
// public keys, from selfupdate-note keygen, for the example repository;
// their signer keys were not kept.
const (
	ciKey     = "github.com/owner/demo/selfupdate+d533e23c+AZWyjkW2R4pNwZVCR12UCgKcvAen9a38ioPLNweII8c2"
	backupKey = "github.com/owner/demo/selfupdate-backup+5c82bb68+AQT/+4AX+ymyQ4mGQ4r3jYdvYUWMOmIMXJ3ovtCn+WWh"
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
	fmt.Println("verifier ready")
	// Output:
	// verifier ready
}

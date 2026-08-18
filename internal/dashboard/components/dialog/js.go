package dialog

import (
	_ "embed"
)

//go:embed dialog.js
var dialogJS []byte

// JS returns the embedded dialog.js script bytes.
func JS() []byte {
	return dialogJS
}

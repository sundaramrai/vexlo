package installers

import _ "embed"

// Embed the same installer scripts that contributors can run from the checkout.
//
//go:embed install.sh
var shell []byte

//go:embed install.ps1
var powershell []byte

func Shell() []byte { return shell }

func PowerShell() []byte { return powershell }

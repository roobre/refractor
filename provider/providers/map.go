package providers

import (
	"roob.re/refractor/provider/providers/archlinux"
	"roob.re/refractor/provider/providers/command"
)
import "roob.re/refractor/provider/types"

// Map contains a list of provider builders given their friendly name.
var Map = map[string]types.Builder{
	"command": {
		DefaultConfig: command.DefaultConfig,
		New:           command.New,
	},
	"archlinux": {
		DefaultConfig: archlinux.DefaultConfig,
		New:           archlinux.New,
	},
}

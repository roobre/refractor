package providers

import "roob.re/shatter/provider/providers/archlinux"
import "roob.re/shatter/provider/types"

// Map contains a list of provider builders given their friendly name.
var Map = map[string]types.Builder{
	"archlinux": {
		DefaultConfig: archlinux.DefaultConfig,
		New:           archlinux.New,
	},
}

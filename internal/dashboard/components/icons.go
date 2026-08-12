package components

import (
	"github.com/a-h/templ"
	"github.com/oniharnantyo/tinyroute/internal/dashboard/components/icon"
)

// Icon wraps templui icon component (Lucide icons).
func Icon(name string, class string) templ.Component {
	if class == "" {
		class = "w-5 h-5"
	}
	return icon.Icon(name)(icon.Props{Class: class})
}

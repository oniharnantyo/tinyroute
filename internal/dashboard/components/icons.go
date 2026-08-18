package components

import (
	"github.com/a-h/templ"
	"github.com/oniharnantyo/tinyroute/internal/dashboard/components/icon"
)

// IconFunc renders an icon with the given props. Every icon in the
// shadcn-templ icon package (e.g. icon.Cpu, icon.Search) has this type,
// so wrapper props can accept any of them by name.
type IconFunc = func(...icon.Props) templ.Component

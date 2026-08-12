// Package core defines the value types and interfaces for tinyroute.
//
// This package MUST NOT import anything outside the Go standard library.
// It is the shared foundation imported by all other internal packages.
// No sibling internal packages may import each other; shared types belong here.
//
// Verify with: go list -deps ./internal/core | grep -v std
package core

#!/usr/bin/env bash
set -euo pipefail

echo "==> Generating templ components..."
if command -v templ &> /dev/null; then
  templ generate
elif [ -f "$(go env GOPATH)/bin/templ" ]; then
  "$(go env GOPATH)/bin/templ" generate
else
  echo "Error: templ binary not found. Run 'go install github.com/a-h/templ/cmd/templ@latest'"
  exit 1
fi

echo "==> Compiling Tailwind CSS..."
if [ -f "./bin/tailwindcss" ]; then
  ./bin/tailwindcss -i internal/dashboard/assets/input.css -o internal/dashboard/assets/styles.css --minify
elif command -v tailwindcss &> /dev/null; then
  tailwindcss -i internal/dashboard/assets/input.css -o internal/dashboard/assets/styles.css --minify
else
  echo "Error: tailwindcss standalone binary not found in ./bin/tailwindcss"
  exit 1
fi

echo "==> Building tinyroute binary..."
go build -o tinyroute main.go

echo "==> Build complete!"

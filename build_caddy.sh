#!/bin/bash

set -euxo pipefail

# Get the Caddy version currenly used for the adapter
CADDY_VERSION=$(go list -m github.com/caddyserver/caddy/v2 | cut -d " " -f 2)

mkdir -p caddy
cd caddy
curl "https://raw.githubusercontent.com/caddyserver/caddy/v2/cmd/caddy/main.go" > main.go
sed -i.bak 's/^)/	_ "github.com\/caddyserver\/nginx-adapter"\'$'\n)/' main.go && rm main.go.bak

go mod init caddy
go mod edit -require=github.com/caddyserver/caddy/v2@"$CADDY_VERSION"
go build -o caddy_v2
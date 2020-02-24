#!/bin/bash

mkdir caddy || true
cd caddy || true
curl "https://raw.githubusercontent.com/caddyserver/caddy/v2/cmd/caddy/main.go" > main.go
sed -i.bak 's/^)/	_ "github.com\/caddyserver\/nginx-adapter"\'$'\n)/' main.go && rm main.go.bak

go mod init caddy
grep 'require' ../go.mod >> go.mod
go build
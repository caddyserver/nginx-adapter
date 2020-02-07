module github.com/caddyserver/nginx-adapter

go 1.13

require (
	github.com/caddyserver/caddy/v2 v2.0.0-beta12
	github.com/cenkalti/backoff/v3 v3.2.1 // indirect
	github.com/miekg/dns v1.1.27 // indirect
	go.uber.org/atomic v1.5.1 // indirect
	go.uber.org/multierr v1.4.0 // indirect
	go.uber.org/zap v1.13.0 // indirect
	golang.org/x/crypto v0.0.0-20200109152110-61a87790db17 // indirect
	golang.org/x/sys v0.0.0-20200107162124-548cf772de50 // indirect
	golang.org/x/tools v0.0.0-20200110042803-e2f26524b78c // indirect
)

replace github.com/caddyserver/caddy/v2 => ../caddy

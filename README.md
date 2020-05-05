Caddy NGINX Config Adapter
==========================

This is a [config adapter](https://caddyserver.com/docs/config-adapters) for Caddy which converts [NGINX config files](https://www.nginx.com/resources/wiki/start/topics/examples/full/) into Caddy's native format.

**This project is not complete, and we are asking the community to help finish its development.** Due to resource constraints, we are unable to do all the development on our own at this time. However, we hope you will pick it up and collaborate on it together as a community. We'll be happy to coordinate efforts from the community. Start by opening issues and pull requests, then reviewing pull requests and testing changes!

Currently supported directives per context:

* main:
  * http
* http:
  * server
* server:
  * listen
  * server_name
  * location
  * root
  * access_log
  * rewrite
  * if
* if:
  * break
  * return
  * rewrite
  * set
* upstream:
  * server
  * hash
  * ip_hash
  * keepalive
  * keepalive_requests
  * keepalive_timeout
  * ntlm
  * least_conn
  * random
* location:
  * location
  * if
  * root
  * add_header
  * deny
  * allow
  * rewrite
  * fastcgi_pass
  * proxy_pass
  * expires
  * return
* if (in location):
  * root
  * gzip
  * add_header
  * expires
  * proxy_pass

Thank you, and we hope you have fun with it!

## Install

First, the [xcaddy](https://github.com/caddyserver/xcaddy) command:

```shell
$ go get -u github.com/caddyserver/xcaddy/cmd/xcaddy
```

Then build Caddy with this Go module plugged in. For example:

```shell
$ xcaddy build --with github.com/caddyserver/nginx-adapter
```

## Use

Using this config adapter is the same as all the other config adapters.

- [Learn about config adapters in the Caddy docs](https://caddyserver.com/docs/config-adapters)
- You can adapt your config with the [`adapt` command](https://caddyserver.com/docs/command-line#caddy-adapt)

You can also run Caddy directly with an nginx config using [`caddy run|start --config nginx.conf --adapter nginx`](https://caddyserver.com/docs/command-line#caddy-run) (however, we do not recommend this until the config adapter is completed, since unfinished directives may just result in warnings and not errors).


## Disclaimer

This project is not affiliated with F5 Networks or NGINX, Inc. NGINX is a registered trademark of NGINX, Inc.

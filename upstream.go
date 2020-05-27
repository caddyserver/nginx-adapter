package nginxconf

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
)

type Upstream struct {
	Servers         reverseproxy.UpstreamPool
	NTLM            bool
	SelectionPolicy struct {
		Name     string
		Selector reverseproxy.Selector
	}
	KeepAlive *reverseproxy.KeepAlive
}

var nginxPolicyToCaddy = map[string]string{
	"random":     "random_choose",
	"least_conn": "least_conn",
	"ip_hash":    "ip_hash",
	"hash":       "header",
}

const unixPrefix = "unix:"

func (ss *setupState) upstreamContext(dirs []Directive) (Upstream, []caddyconfig.Warning, error) {
	var upstream Upstream
	var warns []caddyconfig.Warning
	for _, dir := range dirs {
		switch dir.Name() {
		case "server":
			// From: https://nginx.org/en/docs/http/ngx_http_upstream_module.html
			// The address can be specified as a domain name or IP address, with an optional port,
			// or as a UNIX-domain socket path specified after the “unix:” prefix.
			addr := dir.Param(1)
			var network, hostport string = "tcp", addr
			if strings.HasPrefix(addr, unixPrefix) {
				slicedAddress := strings.Split(addr, "unix:")
				hostport = slicedAddress[1]
				network = "unix" // ends up being "unix"
			}
			host, port, err := net.SplitHostPort(hostport)
			if err != nil {
				warns = append(warns, caddyconfig.Warning{
					File:      dir.File,
					Line:      dir.Line,
					Directive: dir.Name(),
					Message:   fmt.Sprintf("error splitting the host/port of upstream: %s", hostport),
				})
				return upstream, warns, err
			}
			addr = caddy.JoinNetworkAddress(network, host, port)
			u := &reverseproxy.Upstream{Dial: addr}

			if len(dir.Params) > 2 {
				params := dir.Params[2:]
				for _, v := range params {
					if strings.HasPrefix(v, "weight") {
						weight := strings.Split(v, "=")[1]
						w, _ := strconv.ParseInt(weight, 10, 32)
						u.MaxRequests = int(w)
					}
					// TODO: support other flags
				}
			}
			upstream.Servers = append(upstream.Servers, u)
		case "hash":
			upstream.SelectionPolicy.Name = nginxPolicyToCaddy[dir.Name()]
			upstream.SelectionPolicy.Selector = reverseproxy.HeaderHashSelection{
				Field: dir.Param(2),
			}
		case "ip_hash":
			upstream.SelectionPolicy.Name = nginxPolicyToCaddy[dir.Name()]
			upstream.SelectionPolicy.Selector = reverseproxy.IPHashSelection{}
		case "keepalive":
			upstream.KeepAlive = new(reverseproxy.KeepAlive)
			b := true
			upstream.KeepAlive.Enabled = &b
			i, _ := strconv.ParseInt(dir.Param(1), 10, 64)
			upstream.KeepAlive.MaxIdleConns = int(i)
		case "keepalive_requests":
			i, _ := strconv.ParseInt(dir.Param(1), 10, 64)
			upstream.KeepAlive.MaxIdleConnsPerHost = int(i)
		case "keepalive_timeout":
			d, _ := time.ParseDuration(dir.Param(1))
			upstream.KeepAlive.IdleConnTimeout = caddy.Duration(d)
		case "ntlm":
			upstream.NTLM = true
		case "least_conn":
			upstream.SelectionPolicy.Name = nginxPolicyToCaddy[dir.Name()]
			upstream.SelectionPolicy.Selector = reverseproxy.LeastConnSelection{}
		case "random":
			upstream.SelectionPolicy.Name = nginxPolicyToCaddy[dir.Name()]
			upstream.SelectionPolicy.Selector = reverseproxy.RandomChoiceSelection{}
		default:
			warns = append(warns, caddyconfig.Warning{
				File:      dir.File,
				Line:      dir.Line,
				Directive: dir.Name(),
				Message:   ErrUnrecognized,
			})
		}
	}
	return upstream, warns, nil
}

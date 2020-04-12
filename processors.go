package nginxconf

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/fileserver"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/headers"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy/fastcgi"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/rewrite"
	ntlmproxy "github.com/caddyserver/ntlm-transport"
)

var splitPathInfoExtension = regexp.MustCompile(`(\.[[:alnum:]]+)`)

func processAllow(dir Directive) map[string]caddyhttp.RequestMatcher {
	var reqMatcher caddyhttp.RequestMatcher
	var key string
	switch dir.Param(1) {
	case "all":
		reqMatcher = caddyhttp.MatchRemoteIP{
			Ranges: []string{"0.0.0.0:0", "::/0"},
		}
		key = "remote_ip"
	case "unix:":
		reqMatcher = caddyhttp.MatchProtocol("unix")
		key = "protocol"
	default:
		reqMatcher = caddyhttp.MatchRemoteIP{
			Ranges: dir.Params[1:],
		}
		key = "remote_ip"
	}
	matchConfMap := make(map[string]caddyhttp.RequestMatcher)
	matchConfMap[key] = reqMatcher
	return matchConfMap
}

func processDeny(dir Directive) (caddyhttp.Subroute, []caddyconfig.Warning) {
	var warns []caddyconfig.Warning
	var reqMatcher caddyhttp.RequestMatcher
	var key string
	switch dir.Param(1) {
	case "all":
		reqMatcher = caddyhttp.MatchRemoteIP{
			Ranges: []string{"0.0.0.0:0", "::/0"},
		}
		key = "remote_ip"
	case "unix:":
		reqMatcher = caddyhttp.MatchProtocol("unix")
		key = "protocol"
	default:
		reqMatcher = caddyhttp.MatchRemoteIP{
			Ranges: dir.Params[1:],
		}
		key = "remote_ip"
	}

	h := caddyhttp.Subroute{
		Routes: caddyhttp.RouteList{
			caddyhttp.Route{
				Terminal: true,
				HandlersRaw: []json.RawMessage{
					caddyconfig.JSONModuleObject(caddyhttp.StaticResponse{
						StatusCode: caddyhttp.WeakString("403"),
					}, "handler", "static_response", &warns),
				},
				MatcherSetsRaw: []caddy.ModuleMap{
					{
						key: caddyconfig.JSON(reqMatcher, &warns),
					},
				},
			},
		},
	}
	return h, warns
}

// processAddHeader processese the `add_heeader` directive and returns the corresponding the handler *headers.Handler
func processAddHeader(dir Directive) (*headers.Handler, []caddyconfig.Warning) {
	var warns []caddyconfig.Warning
	hdr := new(headers.Handler)

	hdr.Response = &headers.RespHeaderOps{
		HeaderOps: new(headers.HeaderOps),
		Deferred:  true,
	}
	hdr.Response.Set = make(http.Header)
	hdr.Response.Set.Set(dir.Param(1), dir.Param(2))
	if len(dir.Params) == 4 && dir.Param(3) == "always" {
		hdr.Response.Require = new(caddyhttp.ResponseMatcher)
		hdr.Response.Require.StatusCode = []int{200, 201, 204, 206, 301, 302, 303, 304, 307, 308}
		hdr.Response.Require.Headers = http.Header{
			dir.Param(1): {dir.Param(2)},
		}
	}
	return hdr, warns
}

// processExpires processese the `expires` directive and returns the corresponding the handler *headers.Handler
func processExpires(dir Directive) (*headers.Handler, []caddyconfig.Warning) {
	var warns []caddyconfig.Warning
	if len(dir.Params) != 2 {
		warns = append(warns, caddyconfig.Warning{
			File:      dir.File,
			Line:      dir.Line,
			Directive: dir.Name(),
			Message:   "only the form `expires time` (with non-spaced time parameter) is accepted",
		})
		return nil, warns
	}

	hdr := new(headers.Handler)

	hdr.Response = &headers.RespHeaderOps{
		HeaderOps: new(headers.HeaderOps),
		Deferred:  true,
	}
	hdr.Response.Set = make(http.Header)

	var cacheControl string
	arg := dir.Param(1)
	switch arg {
	case "off":
		return nil, nil
	case "-1", "epoch":
		cacheControl = "no-cache"
	case "max":
		// 10 years per nginx docs
		cacheControl = fmt.Sprintf("max-age=%.0f", 315360000.0)
	default:
		if strings.Contains(arg, "@") {
			warns = append(warns, caddyconfig.Warning{
				File:      dir.File,
				Line:      dir.Line,
				Directive: dir.Name(),
				Message:   ErrExpiresAtTime,
			})
			return nil, warns
		}

		var duration time.Duration
		// ref: https://nginx.org/en/docs/http/ngx_http_headers_module.html#expires
		// ref: https://nginx.org/en/docs/syntax.html
		re := regexp.MustCompile(`(\d{1,})(ms|s|m|h|d|w|M|y)`)
		matches := re.FindAllStringSubmatch(arg, -1)
		for i := 0; i < len(matches); i++ {
			amount, err := strconv.ParseInt(matches[i][1], 10, 64)
			if err != nil {
				warns = append(warns, caddyconfig.Warning{
					File:      dir.File,
					Line:      dir.Line,
					Directive: dir.Name(),
					Message:   err.Error(),
				})
				continue
			}
			unit := matches[i][2]
			switch unit {
			case "ms", "s", "m", "h":
				d, err := time.ParseDuration(matches[i][0])
				if err != nil {
					warns = append(warns, caddyconfig.Warning{
						File:      dir.File,
						Line:      dir.Line,
						Directive: dir.Name(),
						Message:   err.Error(),
					})
				}
				duration += d
			case "d":
				duration += (time.Duration(amount) * time.Hour * 24)
			case "M":
				duration += (time.Duration(amount) * time.Hour * 24 * 30)
			case "y":
				duration += (time.Duration(amount) * time.Hour * 24 * 365)
			}
		}
		cacheControl = fmt.Sprintf("max-age=%.0f", duration.Seconds())
	}
	hdr.Response.Set.Set("Cache-Control", cacheControl)
	return hdr, warns
}

func processFastCGIPass(dirs []Directive) (*caddyhttp.Subroute, []caddyconfig.Warning) {
	var warns []caddyconfig.Warning

	// majority fo the code below is copied from:
	// github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy/fastcgi (caddyfile.go)
	// and modified to fit the adapter.

	index := "index.php" // default index
	if v, ok := getDirective(dirs, "fastcgi_index"); ok {
		index = v.Param(1)
	}

	// route to redirect to canonical path if index PHP file
	redirMatcherSet := caddy.ModuleMap{
		"file": caddyconfig.JSON(fileserver.MatchFile{
			TryFiles: []string{"{http.request.uri.path}/" + index},
		}, &warns),
		"not": caddyconfig.JSON(caddyhttp.MatchNot{
			MatcherSetsRaw: []caddy.ModuleMap{
				{
					"path": caddyconfig.JSON(caddyhttp.MatchPath{"*/"}, &warns),
				},
			},
		}, &warns),
	}
	redirHandler := caddyhttp.StaticResponse{
		StatusCode: caddyhttp.WeakString("308"),
		Headers:    http.Header{"Location": []string{"{http.request.uri.path}/"}},
	}
	redirRoute := caddyhttp.Route{
		MatcherSetsRaw: []caddy.ModuleMap{redirMatcherSet},
		HandlersRaw:    []json.RawMessage{caddyconfig.JSONModuleObject(redirHandler, "handler", "static_response", nil)},
	}

	// route to rewrite to PHP index file
	rewriteMatcherSet := caddy.ModuleMap{
		"file": caddyconfig.JSON(fileserver.MatchFile{
			TryFiles: []string{"{http.request.uri.path}", "{http.request.uri.path}/" + index, index},
		}, &warns),
	}
	rewriteHandler := rewrite.Rewrite{
		URI: "{http.matchers.file.relative}",
	}
	rewriteRoute := caddyhttp.Route{
		MatcherSetsRaw: []caddy.ModuleMap{rewriteMatcherSet},
		HandlersRaw:    []json.RawMessage{caddyconfig.JSONModuleObject(rewriteHandler, "handler", "rewrite", nil)},
	}

	extension := []string{".php"}

	// The fastcgi_split_path_info directive takes a regexp with two capture groups,
	// the first capture group points to the script file name and the second is for path info.
	// For example, the regexp for php could be `^(.+.php)(/.*)$`. Caddy splits over the extension. So splitPathInfoExtension
	// finds the extension in the provided input, extract it, and use it for the config.
	if v, ok := getDirective(dirs, "fastcgi_split_path_info"); ok && splitPathInfoExtension.MatchString(v.Param(1)) {
		extension[0] = splitPathInfoExtension.FindStringSubmatch(v.Param(1))[1]
	}

	// route to actually reverse proxy requests to PHP files;
	// match only requests that are for PHP files
	rpMatcherSet := caddy.ModuleMap{
		"path": caddyconfig.JSON([]string{"*" + extension[0]}, &warns),
	}

	// set up the transport for FastCGI, and specifically PHP
	fcgiTransport := fastcgi.Transport{SplitPath: extension}

	// create the reverse proxy handler which uses our FastCGI transport
	rpHandler := &reverseproxy.Handler{
		TransportRaw: caddyconfig.JSONModuleObject(fcgiTransport, "protocol", "fastcgi", nil),
	}

	/***
	* If upstream doesn't have scheme explicitly specified as scheme://host, then
	* assume it's tcp by prefixing it with tcp://. Later we check if the hostname is `unix:`.
	* If so, then the arguemnt of fastcgi_pass was of the form `unix:/some/path`.
	**/
	passDirective, _ := getDirective(dirs, "fastcgi_pass")
	passArgs := passDirective.Param(1)
	if !strings.Contains(passArgs, "://") {
		passArgs = "tcp://" + passArgs
	}
	upstream, err := url.Parse(passArgs)
	if err != nil {
		warns = append(warns, caddyconfig.Warning{
			File:      passDirective.File,
			Line:      passDirective.Line,
			Directive: passDirective.Name(),
			Message:   err.Error(),
		})
		return nil, warns
	}

	var network, host string = upstream.Scheme, upstream.Hostname()

	// the argument of fastcgi_pass could have been of either of these forms:
	// http://unix:/tmp/backend.socket:/uri/ , unix:///some/path
	if upstream.Hostname() == unixPrefix || upstream.Scheme == "unix" {
		network = "unix"
		host = (strings.Split(upstream.Path, ":"))[0]
	}
	rpHandler.Upstreams = append(rpHandler.Upstreams, &reverseproxy.Upstream{Dial: caddy.JoinNetworkAddress(network, host, upstream.Port())})

	// create the final reverse proxy route which is
	// conditional on matching PHP files
	rpRoute := caddyhttp.Route{
		MatcherSetsRaw: []caddy.ModuleMap{rpMatcherSet},
		HandlersRaw:    []json.RawMessage{caddyconfig.JSONModuleObject(rpHandler, "handler", "reverse_proxy", nil)},
	}

	subroute := &caddyhttp.Subroute{
		Routes: caddyhttp.RouteList{redirRoute, rewriteRoute, rpRoute},
	}
	return subroute, warns
}

func processProxyPass(dir Directive, upstreams map[string]Upstream) (*reverseproxy.Handler, []caddyconfig.Warning) {
	var warns []caddyconfig.Warning
	h := &reverseproxy.Handler{
		Headers: &headers.Handler{
			Request: &headers.HeaderOps{
				Set: http.Header{
					"Host": []string{"{http.reverse_proxy.upstream.host}"},
				},
			},
		},
	}
	ur, err := url.Parse(dir.Param(1))
	if err != nil {
		warns = append(warns, caddyconfig.Warning{
			File:      dir.File,
			Line:      dir.Line,
			Directive: dir.Name(),
			Message:   err.Error(),
		})
		return nil, warns
	}
	u, ok := upstreams[ur.Hostname()]
	if !ok { // the specified host wasn't part of any parsed upstreams, so just grab whatever is there
		for _, up := range dir.Params[1:] {
			u, err := url.Parse(up)
			if err != nil {
				warns = append(warns, caddyconfig.Warning{
					File:      dir.File,
					Line:      dir.Line,
					Directive: dir.Name(),
					Message:   err.Error(),
				})
				continue
			}

			// according to the docs, this is how nginx proxy_pass'es to unix socket:
			//
			// `proxy_pass http://unix:/tmp/backend.socket:/uri/;`
			//
			// which results in this url.URL structure:
			//	&url.URL{
			//		Scheme:"http"
			//		Host:"unix:"
			//		Path:"/tmp/backend.socket:/uri/"
			//	}
			//
			// Hence we have to check if the Host is `unix:` to figure upstream is a unix socket.
			network := "tcp"
			host := u.Hostname()
			if u.Hostname() == unixPrefix {
				network = "unix"
				host = (strings.Split(u.Path, ":"))[0]
			}
			h.Upstreams = append(h.Upstreams, &reverseproxy.Upstream{Dial: caddy.JoinNetworkAddress(network, host, u.Port())})
		}
	} else {
		h.Upstreams = u.Servers
		var transport string
		var rt http.RoundTripper
		if u.NTLM {
			transport = "http_ntlm"
			nt := &ntlmproxy.NTLMTransport{
				HTTPTransport: new(reverseproxy.HTTPTransport),
			}
			if ur.Scheme == "https" {
				nt.TLS = new(reverseproxy.TLSConfig)
			}
			nt.KeepAlive = u.KeepAlive
			rt = nt
		} else {
			transport = "http"
			ht := &reverseproxy.HTTPTransport{}
			if ur.Scheme == "https" {
				ht.TLS = new(reverseproxy.TLSConfig)
			}
			ht.KeepAlive = u.KeepAlive
			rt = ht
		}
		h.TransportRaw = caddyconfig.JSONModuleObject(rt, "protocol", transport, nil)
		if u.SelectionPolicy.Name != "" {
			h.LoadBalancing = new(reverseproxy.LoadBalancing)
			h.LoadBalancing.SelectionPolicyRaw = caddyconfig.JSONModuleObject(u.SelectionPolicy.Selector, "policy", u.SelectionPolicy.Name, nil)
		}
	}
	return h, warns
}

// processRewrite returns a Subroute because rewrite require conditional match, and this is attainable
// by detouring the request into a subroute where the `matcher` is controlled.
func processRewrite(dir Directive) (caddyhttp.Subroute, []caddyconfig.Warning) {
	var warns []caddyconfig.Warning
	reqMatcher := caddyhttp.MatchPathRE{
		MatchRegexp: caddyhttp.MatchRegexp{
			Pattern: dir.Param(1),
		},
	}
	rewriteHandler := rewrite.Rewrite{
		URI: dir.Param(2),
	}
	subrouteHandler := caddyhttp.Subroute{
		Routes: caddyhttp.RouteList{
			caddyhttp.Route{
				HandlersRaw: []json.RawMessage{
					caddyconfig.JSONModuleObject(rewriteHandler, "handler", "rewrite", &warns),
				},
				MatcherSetsRaw: []caddy.ModuleMap{
					{
						"path_regexp": caddyconfig.JSON(reqMatcher, &warns),
					},
				},
			},
		},
	}
	return subrouteHandler, warns
}

func processReturn(dir Directive) (caddyhttp.StaticResponse, []caddyconfig.Warning) {
	var warns []caddyconfig.Warning
	arg := dir.Param(1)
	h := caddyhttp.StaticResponse{
		Close: true,
	}

	if isNumeric(arg) {
		h.StatusCode = caddyhttp.WeakString(arg)
		secondArg := dir.Param(2)
		if secondArg != "" {
			u, err := url.Parse(secondArg)
			if err != nil { // if it isn't a URL, then it's a body content
				warns = append(warns, caddyconfig.Warning{
					File:      dir.File,
					Line:      dir.Line,
					Directive: dir.Name(),
					Message:   err.Error(),
				})
				return h, warns
			} else if u.Scheme == "" && u.Host == "" {
				h.Body = secondArg
			} else {
				h.Headers = http.Header{"Location": []string{u.String()}}
			}
		}
	} else {
		h.StatusCode = caddyhttp.WeakString(http.StatusFound)
		h.Headers = http.Header{"Location": []string{arg}}
	}
	return h, warns
}

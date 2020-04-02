package nginxconf

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/encode"
	caddygzip "github.com/caddyserver/caddy/v2/modules/caddyhttp/encode/gzip"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/fileserver"
)

func (ss *setupState) ifContext(dirs []Directive) ([]json.RawMessage, []caddyconfig.Warning) {
	var warnings []caddyconfig.Warning
	var handlers []json.RawMessage
	for _, dir := range dirs {
		var warns []caddyconfig.Warning
		switch dir.Name() {
		case "break":
			h := caddyhttp.StaticResponse{
				Close: true,
			}
			encodedHandler := caddyconfig.JSONModuleObject(h, "handler", "static_response", &warns)
			handlers = append(handlers, encodedHandler)
		case "return":
			h, w := processReturn(dir)
			warns = append(warns, w...)
			encodedHandler := caddyconfig.JSONModuleObject(h, "handler", "static_response", &warns)
			handlers = append(handlers, encodedHandler)
		case "rewrite":
			h, w := processRewrite(dir)
			warns = append(warns, w...)
			encodedHandler := caddyconfig.JSONModuleObject(h, "handler", "subroute", &warns)
			handlers = append(handlers, encodedHandler)
		case "set":
			h := caddyhttp.VarsMiddleware{
				dir.Param(1): dir.Param(2),
			}
			handlers = append(handlers, caddyconfig.JSONModuleObject(h, "handler", "vars", &warns))
		default:
			warns = append(warns, caddyconfig.Warning{
				File:      dir.File,
				Line:      dir.Line,
				Directive: dir.Name(),
				Message:   ErrUnrecognized,
			})
		}
		warnings = append(warnings, warns...)
	}
	return handlers, warnings
}

func (ss *setupState) ifInLocationContext(dirs []Directive) ([]json.RawMessage, []caddyconfig.Warning) {
	var warnings []caddyconfig.Warning
	var handlers []json.RawMessage
	for _, dir := range dirs {
		var warns []caddyconfig.Warning
		switch dir.Name() {
		case "root":
			fileServer := fileserver.FileServer{
				Root: dir.Param(1),
				// TODO: all remaining fields...
			}
			handlers = append(handlers, caddyconfig.JSONModuleObject(fileServer, "handler", "file_server", &warns))
		case "fastcgi_pass":
			h, w := processFastCGIPass([]Directive{dir})
			warns = append(warns, w...)
			handlers = append(handlers, caddyconfig.JSONModuleObject(h, "handler", "subroute", &warns))
		case "gzip":
			h := encode.Encode{
				EncodingsRaw: caddy.ModuleMap{
					"gzip": caddyconfig.JSON(caddygzip.Gzip{}, nil),
				},
			}
			handlers = append(handlers, caddyconfig.JSONModuleObject(h, "handler", "encode", &warns))
		case "add_header":
			hdr, w := processAddHeader(dir)
			warns = append(warns, w...)
			handlers = append(handlers, caddyconfig.JSONModuleObject(hdr, "handler", "headers", &warns))
		case "expires":
			hdr, w := processExpires(dir)
			warns = append(warns, w...)
			handlers = append(handlers, caddyconfig.JSONModuleObject(hdr, "handler", "headers", &warns))
		case "proxy_pass":
			h, w := processProxyPass(dir, ss.upstreams)
			warns = append(warns, w...)
			handlers = append(handlers, caddyconfig.JSONModuleObject(h, "handler", "reverse_proxy", &warns))
		default:
			warns = append(warns, caddyconfig.Warning{
				File:      dir.File,
				Line:      dir.Line,
				Directive: dir.Name(),
				Message:   ErrUnrecognized,
			})
		}
		warnings = append(warnings, warns...)
	}
	return handlers, warnings
}

func calculateIfMatcher(dir Directive) (caddy.ModuleMap, []caddyconfig.Warning) {
	var warns []caddyconfig.Warning
	var routeMatcher caddy.ModuleMap

	switch len(dir.Params) {
	case 2: // something like this: if ($invalid_referer)
		arg := strings.Trim(dir.Param(1), "()")
		routeMatcher = caddy.ModuleMap{
			"vars": caddyconfig.JSON(caddyhttp.VarsMatcher{getCaddyVar(arg): "true"}, &warns),
		}
	case 4: // something like this: if ($http_cookie ~* "id=([^;]+)(?:;|$)")
		loperand, op, roperand := strings.TrimLeft(dir.Param(1), "("), dir.Param(2), strings.TrimRight(dir.Param(3), ")")
		switch op {
		case "=":
			// Caddy sets a collection of HTTP variables to the request context, so the VarMatcher
			// as wildcard matcher.
			// https://github.com/caddyserver/caddy/blob/271b5af14894a8cca5fc6aa6f1c17823a1fb5ff3/modules/caddyhttp/server.go#L139
			routeMatcher = caddy.ModuleMap{
				"vars": caddyconfig.JSON(caddyhttp.VarsMatcher{getCaddyVar(loperand): roperand}, &warns),
			}
		case "~", "!~", "~*", "!~*": // regexps
			pattern := roperand
			if strings.HasSuffix(pattern, "*") {
				pattern = "(?i)" + pattern // case-insensitive matching
			}
			routeMatcher = caddy.ModuleMap{
				"vars_regexp": caddyconfig.JSON(caddyhttp.MatchVarsRE{
					getCaddyVar(loperand): &caddyhttp.MatchRegexp{
						Pattern: pattern,
					},
				}, &warns),
			}
			if op == "!~" || op == "!~*" {
				routeMatcher = caddy.ModuleMap{
					"not": caddyconfig.JSON(caddyhttp.MatchNot{
						MatcherSetsRaw: []caddy.ModuleMap{
							routeMatcher,
						},
					}, &warns),
				}
			}
		default:
			warns = append(warns, caddyconfig.Warning{
				File:      dir.File,
				Line:      dir.Line,
				Directive: dir.Name(),
				Message:   fmt.Sprintf("unsupported `if` operator: %s", dir.Param(2)),
			})
			return nil, warns
		}
	default:
		warns = append(warns, caddyconfig.Warning{
			File:      dir.File,
			Line:      dir.Line,
			Directive: dir.Name(),
			Message:   fmt.Sprintf("unsupported count of `if` arguments: %v", dir.Params),
		})
		return nil, warns
	}
	return routeMatcher, warns
}

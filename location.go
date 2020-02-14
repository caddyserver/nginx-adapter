package nginxconf

import (
	"encoding/json"
	"strings"

	"github.com/caddyserver/caddy/v2/caddyconfig"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/fileserver"
)

// locationContext processes the `location` directive in isolation from its surrounding
// expecting the caller to handle it as `subroute`
func (ss *setupState) locationContext(rootMatcher map[string]caddyhttp.RequestMatcher, dirs []Directive) (caddyhttp.RouteList, []caddyconfig.Warning, error) {
	var warnings []caddyconfig.Warning
	var handlers []json.RawMessage

	currentMatcherSet := []map[string]caddyhttp.RequestMatcher{rootMatcher}

nextDirective:
	for _, dir := range dirs {
		var warns []caddyconfig.Warning

		switch dir.Name() {
		case "location": // deal with devils first
			matchConfMap := make(map[string]caddyhttp.RequestMatcher)

			if len(dir.Params) > 2 {
				switch dir.Param(1) {
				case "=":
					matchConfMap["path"] = caddyhttp.MatchPath(dir.Params[2:])
				case "~", "~*": // treat both as regexp matchers
					pattern := dir.Param(2)
					if strings.HasSuffix(pattern, "*") {
						pattern = "(?i)" + pattern // case-insensitive matching
					}
					matchConfMap["path_regexp"] = caddyhttp.MatchPathRE{
						MatchRegexp: caddyhttp.MatchRegexp{
							Pattern: pattern,
						},
					}
				case "^~":
					/*
						What it does is... if it is matched, then no regular expression locations will try to be matched.
						It basically terminates location block matching.
						https://www.keycdn.com/support/nginx-location-directive
					*/
					matchConfMap["path"] = caddyhttp.MatchPath([]string{dir.Param(2) + "*"})
					warns = append(warns, caddyconfig.Warning{
						File:      dir.File,
						Line:      dir.Line,
						Directive: dir.Name(),
						Message:   "the adapter treats the ^~ location modifier as prefix match only, with no prioritization",
					})
				}
			} else if len(dir.Params) == 2 { // only path
				if strings.HasPrefix(dir.Param(1), "@") {
					warnings = append(warnings, caddyconfig.Warning{
						File:      dir.File,
						Line:      dir.Line,
						Directive: dir.Name(),
						Message:   ErrNamedLocation,
					})
					continue nextDirective
				}
				// append wild character because nginx treat naked path matchers as prefix matchers
				matchConfMap["path"] = caddyhttp.MatchPath([]string{dir.Param(1) + "*"})
			}
			subsubroutes, warns, err := ss.locationContext(matchConfMap, dir.Block)
			if err != nil || len(subsubroutes) == 0 {
				warnings = append(warnings, warns...)
				return nil, warnings, err
			}
			h := caddyhttp.Subroute{
				Routes: subsubroutes,
			}
			handlers = append(handlers, caddyconfig.JSONModuleObject(h, "handler", "subroute", &warns))
		case "if":
			matcher, w := calculateIfMatcher(dir)
			warns = append(warns, w...)
			if matcher == nil { // warning of failures already appended
				continue nextDirective
			}
			h, w := ss.ifInLocationContext(dir.Block)
			warns = append(warns, w...)
			sroute := caddyhttp.Subroute{
				Routes: []caddyhttp.Route{
					{
						MatcherSetsRaw: caddyhttp.RawMatcherSets{matcher},
						HandlersRaw:    h,
					},
				},
			}
			handlers = append(handlers, caddyconfig.JSONModuleObject(sroute, "handler", "subroute", &warns))
		case "root":
			fileServer := fileserver.FileServer{
				Root: dir.Param(1),
				// TODO: all remaining fields...
			}
			handlers = append(handlers, caddyconfig.JSONModuleObject(fileServer, "handler", "file_server", &warns))
		case "add_header":
			hdr, w := processAddHeader(dir)
			warns = append(warns, w...)
			handlers = append(handlers, caddyconfig.JSONModuleObject(hdr, "handler", "headers", &warns))
		case "deny":
			h, w := processDeny(dir)
			warns = append(warns, w...)
			handlers = append(handlers, caddyconfig.JSONModuleObject(h, "handler", "subroute", &warns))
		case "allow":
			currentMatcherSet = append(currentMatcherSet, processAllow(dir))
		case "rewrite":
			h, w := processRewrite(dir)
			warns = append(warns, w...)
			encodedHandler := caddyconfig.JSONModuleObject(h, "handler", "subroute", &warns)
			handlers = append(handlers, encodedHandler)
		case "fastcgi_split_path_info", "fastcgi_index": // only processed if fastcgi_pass is available, so don't react to them here.
		case "fastcgi_pass":
			supportedDirectives := []string{"fastcgi_split_path_info", "fastcgi_index"}
			fcgiDirs := []Directive{dir}
			for _, v := range supportedDirectives {
				fcgiDirs = append(fcgiDirs, getAllDirectives(dirs, v)...)
			}
			h, w := processFastCGIPass(fcgiDirs)
			warns = append(warns, w...)
			handlers = append(handlers, caddyconfig.JSONModuleObject(h, "handler", "subroute", &warns))
		case "proxy_pass":
			h, w := processProxyPass(dir, ss.upstreams)
			warns = append(warns, w...)
			handlers = append(handlers, caddyconfig.JSONModuleObject(h, "handler", "reverse_proxy", &warns))
		case "expires":
			hdr, w := processExpires(dir)
			warns = append(warns, w...)
			if hdr != nil {
				handlers = append(handlers, caddyconfig.JSONModuleObject(hdr, "handler", "headers", &warns))
			}
		case "return":
			h, w := processReturn(dir)
			warns = append(warns, w...)
			encodedHandler := caddyconfig.JSONModuleObject(h, "handler", "static_response", &warns)
			handlers = append(handlers, encodedHandler)
		default:
			warns = append(warns, caddyconfig.Warning{
				File:      dir.File,
				Line:      dir.Line,
				Directive: dir.Name(),
				Message:   ErrUnrecognized,
			})
		}
	}

	r := caddyhttp.Route{}
	var err error
	// set the matcher to route here so the `allow` and `deny` directives get to append their
	// filters if used
	r.MatcherSetsRaw, err = encodeMatcherSets(currentMatcherSet)
	if err != nil {
		// TODO:
		return caddyhttp.RouteList{r}, warnings, err
	}
	r.HandlersRaw = handlers

	return caddyhttp.RouteList{r}, warnings, nil
}

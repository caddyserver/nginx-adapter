package nginxconf

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/fileserver"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/rewrite"
)

func (ss *setupState) serverContext(dirs []Directive) ([]caddyconfig.Warning, error) {
	var warnings []caddyconfig.Warning

	srv := new(caddyhttp.Server)
	srvName := "server_" + strconv.Itoa(len(ss.servers))
	route := caddyhttp.Route{}
	// slice of maps
	var matcherSets []map[string]caddyhttp.RequestMatcher
	var hostMatcher map[string]caddyhttp.RequestMatcher
	var logName string
	var hosts []string

nextDirective:
	for _, dir := range dirs {
		var warns []caddyconfig.Warning
		switch dir.Name() {
		case "listen":
			addr := dir.Param(1)
			if strings.HasPrefix(addr, "unix:") {
				// unix socket
				addr = "unix/" + addr[5:]
			} else if isNumeric(addr) {
				// port only
				addr = ":" + addr
			}

			// see if existing server has this address, and if so, use
			// it; Caddy does not allow servers to have overlapping
			// listener addresses
			for otherSrvName, otherSrv := range ss.servers {
				for _, otherAddr := range otherSrv.Listen {
					if addr == otherAddr {
						srv = otherSrv
						srvName = otherSrvName
						continue nextDirective
					}
				}
			}

			srv.Listen = append(srv.Listen, addr)
		case "server_name":
			hostMatcher = map[string]caddyhttp.RequestMatcher{
				"host": caddyhttp.MatchHost(dir.Params[1:]),
			}
			matcherSets = append(matcherSets, hostMatcher)
			hosts = append(hosts, dir.Params[1:]...)
		case "location":
			var matcher caddyhttp.RequestMatcher
			matchConfMap := make(map[string]caddyhttp.RequestMatcher)

			if len(dir.Params) > 2 {
				switch dir.Param(1) {
				case "=":
					matchConfMap["path"] = caddyhttp.MatchPath(dir.Params[2:])
				case "~", "~*":
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
				matcher = caddyhttp.MatchPath([]string{dir.Param(1) + "*"})
				matchConfMap["path"] = matcher
			}

			locationMatcherSet := append(matcherSets[:], matchConfMap)
			subroutes, warns, err := ss.locationContext(matchConfMap, dir.Block)
			if err != nil || len(subroutes) == 0 {
				warnings = append(warnings, warns...)
				return warnings, err
			}
			var matcherSetsEnc caddyhttp.RawMatcherSets
			// encode the matchers then set the result as raw matcher config
			matcherSetsEnc, err = encodeMatcherSets(locationMatcherSet)
			if err != nil {
				warnings = append(warnings, warns...)
				return warnings, err
			}
			// set the matcher to route
			route.MatcherSetsRaw = matcherSetsEnc

			h := caddyhttp.Subroute{
				Routes: subroutes,
			}
			route.HandlersRaw = []json.RawMessage{
				caddyconfig.JSONModuleObject(h, "handler", "subroute", &warns),
			}

			// append the route
			srv.Routes = append(srv.Routes, route)

			// empty the route for next iteration
			route = caddyhttp.Route{}
		case "root":
			route.MatcherSetsRaw = []caddy.ModuleMap{
				{
					"host": caddyconfig.JSON(hostMatcher["host"], &warns),
				},
			}
			fileServer := fileserver.FileServer{
				Root: dir.Param(1),
				// TODO: all remaining fields...
			}

			route.HandlersRaw = append(route.HandlersRaw,
				caddyconfig.JSONModuleObject(fileServer, "handler", "file_server", &warns),
			)
			// append the route
			srv.Routes = append(srv.Routes, route)
		case "access_log":
			if dir.Param(1) == "off" {
				continue nextDirective
			}

			// just mark the variable
			logName = dir.Param(1)
		case "rewrite":
			reqMatcher := caddyhttp.MatchPathRE{
				MatchRegexp: caddyhttp.MatchRegexp{
					Pattern: dir.Param(1),
				},
			}
			rewriteHandler := rewrite.Rewrite{
				URI: dir.Param(2),
			}
			route.MatcherSetsRaw = []caddy.ModuleMap{
				{
					"path_regexp": caddyconfig.JSON(reqMatcher, &warns),
				},
			}
			route.HandlersRaw = []json.RawMessage{
				caddyconfig.JSONModuleObject(rewriteHandler, "handler", "rewrite", &warns),
			}

			// append the route
			srv.Routes = append(srv.Routes, route)

			// empty the route for next iteration
			route = caddyhttp.Route{}
		case "if":
			matcher, w := calculateIfMatcher(dir)
			warns = append(warns, w...)
			if matcher == nil { // warning of failures already appended
				continue nextDirective
			}
			route.MatcherSetsRaw = []caddy.ModuleMap{matcher}
			hs, w := ss.ifContext(dir.Block)
			route.HandlersRaw = hs

			// append the route
			srv.Routes = append(srv.Routes, route)

			// empty the route for next iteration
			route = caddyhttp.Route{}
			warns = append(warns, w...)
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

	if !(route).Empty() {
		srv.Routes = append(srv.Routes, route)
	}

	if logName != "" {
		loggerName := srvName + "_log"
		fileWriter := map[string]interface{}{
			"filename": logName,
		}
		loggerJSON := caddyconfig.JSONModuleObject(fileWriter, "output", "file", &warnings)

		if ss.mainConfig.Logging == nil {
			ss.mainConfig.Logging = &caddy.Logging{
				Logs: make(map[string]*caddy.CustomLog),
			}
		}
		ss.mainConfig.Logging.Logs[loggerName] = &caddy.CustomLog{
			WriterRaw: loggerJSON,
		}

		if srv.Logs == nil {
			srv.Logs = &caddyhttp.ServerLogConfig{
				LoggerNames: make(map[string]string),
			}
		}
		for _, v := range hosts {
			srv.Logs.LoggerNames[v] = loggerName
		}
	}
	ss.servers[srvName] = srv

	return warnings, nil
}

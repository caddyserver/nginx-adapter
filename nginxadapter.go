// Copyright 2015 Matthew Holt and The Caddy Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package nginxconf

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/fileserver"
)

func init() {
	caddyconfig.RegisterAdapter("nginx", Adapter{})
}

// Adapter adapts NGINX config to Caddy JSON.
type Adapter struct{}

// Adapt converts the NGINX config in body to Caddy JSON.
func (Adapter) Adapt(body []byte, options map[string]interface{}) ([]byte, []caddyconfig.Warning, error) {
	tokens := tokenize(body)
	dirs, err := parse(tokens)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing: %v", err)
	}

	// for _, dir := range dirs {
	// 	log.Printf("%v %+v", dir.Params, dir.Block)
	// }

	ss := setupState{
		servers: make(map[string]*caddyhttp.Server),
	}

	warnings, err := ss.mainContext(dirs)
	if err != nil {
		return nil, nil, err
	}

	httpApp := caddyhttp.App{
		Servers: ss.servers,
	}

	ss.mainConfig.AppsRaw = map[string]json.RawMessage{
		"http": caddyconfig.JSON(httpApp, &warnings),
	}

	marshalFunc := json.Marshal
	if options["pretty"] == "true" {
		marshalFunc = caddyconfig.JSONIndent
	}
	result, err := marshalFunc(ss.mainConfig)

	return result, warnings, err
}

type setupState struct {
	mainConfig caddy.Config
	servers    map[string]*caddyhttp.Server
}

func (ss *setupState) mainContext(dirs []Directive) ([]caddyconfig.Warning, error) {
	var warnings []caddyconfig.Warning
	for _, dir := range dirs {
		var warns []caddyconfig.Warning
		var err error
		switch dir.Name() {
		case "http":
			warns, err = ss.httpContext(dir.Block)
		default:
			warns = []caddyconfig.Warning{
				{
					File:      dir.File,
					Line:      dir.Line,
					Directive: dir.Name(),
					Message:   "unrecognized",
				},
			}
		}
		warnings = append(warnings, warns...)
		if err != nil {
			return warnings, err
		}
	}
	return warnings, nil
}

func (ss *setupState) httpContext(dirs []Directive) ([]caddyconfig.Warning, error) {
	var warnings []caddyconfig.Warning
	for _, dir := range dirs {
		var warns []caddyconfig.Warning
		var err error
		switch dir.Name() {
		case "server":
			warns, err = ss.serverContext(dir.Block)
		default:
			warns = []caddyconfig.Warning{
				{
					File:      dir.File,
					Line:      dir.Line,
					Directive: dir.Name(),
					Message:   "unrecognized",
				},
			}
		}
		warnings = append(warnings, warns...)
		if err != nil {
			return warnings, err
		}
	}
	return warnings, nil
}

func (ss *setupState) serverContext(dirs []Directive) ([]caddyconfig.Warning, error) {
	var warnings []caddyconfig.Warning

	srv := new(caddyhttp.Server)
	srvName := "server_" + strconv.Itoa(len(ss.servers))
	var route caddyhttp.Route

nextDirective:
	for _, dir := range dirs {
		var warns []caddyconfig.Warning
		var err error
		switch dir.Name() {
		case "listen":
			addr := dir.Params[1]
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
			matcherSets := []map[string]caddyhttp.RequestMatcher{
				{
					"host": caddyhttp.MatchHost(dir.Params[1:]),
				},
			}
			var matcherSetsEnc []map[string]json.RawMessage
			for _, ms := range matcherSets {
				msEncoded := make(map[string]json.RawMessage)
				for matcherName, val := range ms {
					jsonBytes, err := json.Marshal(val)
					if err != nil {
						return nil, fmt.Errorf("marshaling matcher set %#v: %v", matcherSets, err)
					}
					msEncoded[matcherName] = jsonBytes
				}
				matcherSetsEnc = append(matcherSetsEnc, msEncoded)
			}
			route.MatcherSetsRaw = matcherSetsEnc

		case "root":
			fileServer := fileserver.FileServer{
				Root: dir.Param(1),
				// TODO: all remaining fields...
			}

			route.HandlersRaw = []json.RawMessage{
				caddyconfig.JSONModuleObject(fileServer, "handler", "file_server", &warns),
			}

		default:
			warns = []caddyconfig.Warning{
				{
					File:      dir.File,
					Line:      dir.Line,
					Directive: dir.Name(),
					Message:   "unrecognized",
				},
			}
		}
		warnings = append(warnings, warns...)
		if err != nil {
			return warnings, err
		}
	}

	if !route.Empty() {
		srv.Routes = append(srv.Routes, route)
	}

	ss.servers[srvName] = srv

	return warnings, nil
}

func isNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

// Interface guard
var _ caddyconfig.Adapter = (*Adapter)(nil)

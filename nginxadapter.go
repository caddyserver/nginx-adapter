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
)

func init() {
	caddyconfig.RegisterAdapter("nginx", Adapter{})
}

const ErrUnrecognized = "unrecognized or unsupported nginx directive"
const ErrNamedLocation = "named locations marked by @ are unnsupported"
const ErrExpiresAtTime = "usage of `expires @time` is not supported"

// Adapter adapts NGINX config to Caddy JSON.
type Adapter struct{}

// Adapt converts the NGINX config in body to Caddy JSON.
func (Adapter) Adapt(body []byte, options map[string]interface{}) ([]byte, []caddyconfig.Warning, error) {
	tokens := tokenize(body)
	dirs, err := parse(tokens)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing: %v", err)
	}

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

	upstreams map[string]Upstream
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
					Message:   ErrUnrecognized,
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
		case "upstream":
			up, w, err := ss.upstreamContext(dir.Block)
			warns = append(warns, w...)
			if err != nil {
				return warns, err
			}
			if ss.upstreams == nil {
				ss.upstreams = make(map[string]Upstream)
			}
			ss.upstreams[dir.Param(1)] = up
		default:
			warns = []caddyconfig.Warning{
				{
					File:      dir.File,
					Line:      dir.Line,
					Directive: dir.Name(),
					Message:   ErrUnrecognized,
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

var nginxToCaddyVars = map[string]string{
	"$host:$port":     "{http.request.hostport}",
	"$hostname:$port": "{http.request.hostport}",
	"$host":           "{http.request.host}",
	"$hostname":       "{http.request.host}",
	"$server_port":    "{http.request.port}",
	"$scheme":         "{http.request.scheme}",
	"$request_uri":    "{http.request.uri}",
	"$query_string":   "{http.request.uri.query_string}",
	"$args":           "{http.request.uri.query_string}",
	"$request_method": "{http.request.method}",
}

func getCaddyVar(nginxVar string) string {
	if v, ok := nginxToCaddyVars[nginxVar]; ok {
		return v
	}
	if strings.HasPrefix(nginxVar, "$cookie_") {
		return fmt.Sprintf("{http.request.cookie.%s}", strings.TrimPrefix(nginxVar, "$cookie_"))
	}
	// variables prefixed with `$http_` correspond to respective header field with the suffix name
	// Source: https://nginx.org/en/docs/http/ngx_http_core_module.html#var_http_
	if strings.HasPrefix(nginxVar, "$http_") {
		return fmt.Sprintf("{http.request.header.%s}", strings.TrimPrefix(nginxVar, "$header_"))
	}
	return fmt.Sprintf("{http.vars.%s}", strings.TrimPrefix(nginxVar, "$"))
}

func encodeMatcherSets(currentMatcherSet []map[string]caddyhttp.RequestMatcher) (caddyhttp.RawMatcherSets, error) {
	// encode the matchers then set the result as raw matcher config
	var matcherSetsEnc caddyhttp.RawMatcherSets
	for _, ms := range currentMatcherSet {
		msEncoded := make(map[string]json.RawMessage)
		for matcherName, val := range ms {
			jsonBytes, err := json.Marshal(val)
			if err != nil {
				return nil, fmt.Errorf("marshaling matcher set %#v: %v", currentMatcherSet, err)
			}
			msEncoded[matcherName] = jsonBytes
		}
		matcherSetsEnc = append(matcherSetsEnc, msEncoded)
	}
	return matcherSetsEnc, nil
}

func isNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

// Interface guard
var _ caddyconfig.Adapter = (*Adapter)(nil)

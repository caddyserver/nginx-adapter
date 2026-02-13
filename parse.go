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
	"bytes"
	"fmt"
	"io"
	"os"

	crossplane "github.com/nginxinc/nginx-go-crossplane"
)

// parseNginxConfig uses the crossplane parser to parse NGINX configuration.
// The body parameter contains the raw config bytes, and filename is used
// as the source file name. Included files are resolved from disk.
func parseNginxConfig(body []byte, filename string) ([]Directive, error) {
	payload, err := crossplane.Parse(filename, &crossplane.ParseOptions{
		// Use a custom Open function so the main config body is read from
		// the provided bytes instead of from disk. Included files are
		// opened normally from the filesystem.
		Open: func(path string) (io.ReadCloser, error) {
			if path == filename {
				return io.NopCloser(bytes.NewReader(body)), nil
			}
			return os.Open(path)
		},
		// Combine all included files into a single config tree so the
		// caller sees a unified set of directives.
		CombineConfigs: true,
		// Skip directive validation because the adapter handles
		// unrecognized directives by emitting warnings rather than errors.
		SkipDirectiveContextCheck: true,
		SkipDirectiveArgsCheck:    true,
	})
	if err != nil {
		return nil, fmt.Errorf("crossplane parse: %v", err)
	}

	if len(payload.Config) == 0 {
		return nil, nil
	}

	return convertDirectives(payload.Config[0].Parsed), nil
}

// convertDirectives converts crossplane Directives into the adapter's Directive type.
func convertDirectives(cpDirs crossplane.Directives) []Directive {
	var dirs []Directive
	for _, cpDir := range cpDirs {
		if cpDir.IsComment() {
			continue
		}

		dir := Directive{
			// Params combines the directive name and its arguments into a
			// single slice, matching the convention used by the rest of
			// the adapter code (Params[0] is the name).
			Params: append([]string{cpDir.Directive}, cpDir.Args...),
			File:   cpDir.File,
			Line:   cpDir.Line,
		}

		if cpDir.IsBlock() {
			dir.Block = convertDirectives(cpDir.Block)
		}

		dirs = append(dirs, dir)
	}
	return dirs
}

// Directive represents an nginx configuration directive.
type Directive struct {
	// Params contains the name and parameters on
	// the line. The first element is the name.
	Params []string

	// Block contains the block contents, if present.
	Block []Directive

	File string

	Line int
}

// Name returns the value of the first parameter.
func (d Directive) Name() string {
	return d.Param(0)
}

// Param returns the parameter at position idx.
func (d Directive) Param(idx int) string {
	if idx < len(d.Params) {
		return d.Params[idx]
	}
	return ""
}

func getDirective(dirs []Directive, name string) (Directive, bool) {
	for _, dir := range dirs {
		if dir.Name() == name {
			return dir, true
		}
	}
	return Directive{}, false
}

func getAllDirectives(dirs []Directive, name string) []Directive {
	var matched []Directive
	for _, dir := range dirs {
		if dir.Name() == name {
			matched = append(matched, dir)
		}
	}
	return matched
}

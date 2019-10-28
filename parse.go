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
	"io"
)

func parse(tokens []token) ([]Directive, error) {
	parser := nginxParser{tokens: tokens}
	return parser.nextBlock()
}

type nginxParser struct {
	tokens []token
	cursor int // incrementing this is analogous to consuming the token
}

// nextBlock returns the next block of directives.
func (p *nginxParser) nextBlock() ([]Directive, error) {
	var dirs []Directive
	for {
		// if we've reached end of input (main context)
		// or the end of current block, we're done
		if p.cursor >= len(p.tokens) {
			break
		}
		if p.tokens[p.cursor].text == "}" {
			p.cursor++
			break
		}

		dir, err := p.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, dir)
	}
	return dirs, nil
}

// next returns the next directive.
func (p *nginxParser) next() (Directive, error) {
	if p.cursor == len(p.tokens) {
		return Directive{}, io.EOF
	}

	dir := Directive{
		File: p.tokens[p.cursor].file,
		Line: p.tokens[p.cursor].line,
	}
	for ; p.cursor < len(p.tokens); p.cursor++ {
		tkn := p.tokens[p.cursor]
		if tkn.text == ";" {
			p.cursor++
			break
		}
		if tkn.text == "{" {
			p.cursor++
			dirs, err := p.nextBlock()
			if err != nil {
				return Directive{}, err
			}
			dir.Block = dirs
			break
		}
		dir.Params = append(dir.Params, tkn.text)
	}

	return dir, nil
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

/*
	From: http://nginx.org/en/docs/beginners_guide.html

	nginx consists of modules which are controlled by directives specified in the configuration file.
	Directives are divided into simple directives and block directives. A simple directive consists of
	the name and parameters separated by spaces and ends with a semicolon (;). A block directive has
	the same structure as a simple directive, but instead of the semicolon it ends with a set of
	additional instructions surrounded by braces ({ and }). If a block directive can have other
	directives inside braces, it is called a context (examples: events, http, server, and location).

	Directives placed in the configuration file outside of any contexts are considered to be in the main
	context. The events and http directives reside in the main context, server in http, and location in
	server.

	The rest of a line after the # sign is considered a comment.
*/

/*
	other references:
	https://github.com/recoye/config
	https://github.com/yangchenxing/go-nginx-conf-parser
	https://www.nginx.com/resources/wiki/start/topics/examples/full/
*/

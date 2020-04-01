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
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func parse(tokens []token) ([]Directive, error) {
	parser := nginxParser{tokens: tokens}
	return parser.nextBlock()
}

type nginxParser struct {
	tokens []token
	cursor int // incrementing this is analogous to consuming the token
}

func (p *nginxParser) currentToken() token {
	return p.tokens[p.cursor]
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
		if tkn.text == "include" {
			p.cursor++
			err := p.doInclude()
			if err != nil {
				return Directive{}, err
			}
			tkn = p.tokens[p.cursor]
			dir.File = tkn.file
			dir.Line = tkn.line
		}
		dir.Params = append(dir.Params, tkn.text)
	}
	return dir, nil
}

// Reference: https://wiki.debian.org/Nginx/DirectoryStructure
const nginxConfPrefix = "/etc/nginx"

var nginxConfDirs = []string{
	"conf.d/",
	"modules-available/",
	"modules-enabled/",
	"sites-available/",
	"sites-enabled/",
	"snippets/",
}
var nginxStdConfs = []string{
	"fastcgi.conf",
	"fastcgi_params",
	"koi-utf",
	"koi-win",
	"mime.types",
	"nginx.conf",
	"proxy_params",
	"scgi_params",
	"uwsgi_params",
	"win-utf",
}

// TODO: support relative path includes
func (p *nginxParser) doInclude() error {
	includeToken := p.currentToken()
	includeArg := filepath.Clean(includeToken.text)

	if strings.Count(includeArg, "*") > 1 || strings.Count(includeArg, "?") > 1 ||
		(strings.Contains(includeArg, "[") && strings.Contains(includeArg, "]")) {
		// See issue #2096 - a pattern with many glob expansions can hang for too long
		return fmt.Errorf("Glob pattern may only contain one wildcard (*), but has others: %s", includeArg)
	}
	var importedFiles []string
	if filepath.IsAbs(includeArg) {
		matches, err := filepath.Glob(includeArg)
		if err != nil {
			return err
		}
		for _, v := range matches {
			if _, err := os.Stat(v); !os.IsNotExist(err) {
				importedFiles = append(importedFiles, v)
			}
		}
	}

	// if not absolute, we'll only support including files within /etc/nginx/.
	if len(importedFiles) == 0 {
		// is it one of the standard files?
		for _, v := range nginxStdConfs {
			if v == includeArg {
				importedFiles = append(importedFiles, filepath.Join(nginxConfPrefix, v))
				break
			}
		}
		// by here it is not one of the direct descendant files of /etc/nginx/.
		// Is it in one of the subdirectories?
		//
		// The `filepath.HasPrefix` is bad before it just does `strings.HasPrefix` and
		// doesn't respoect directories boundaries.
		for _, v := range nginxConfDirs {
			testablePath := filepath.Join(nginxConfPrefix, v, includeArg)
			// The argument could have a glob (e.g. custom-*.conf), so expand the glob.
			matches, err := filepath.Glob(filepath.Clean(testablePath))
			if err != nil {
				return err // this is bad glob pattern, so nothing can be done except return
			}
			importedFiles = append(importedFiles, matches...)
		}
	}

	var importedTokens []token
	for _, importFile := range importedFiles {
		newTokens, err := p.doSingleInclude(importFile)
		if err != nil {
			return err
		}
		importedTokens = append(importedTokens, newTokens...)
	}

	// splice out the import directive and its argument (2 tokens total)
	tokensBefore := p.tokens[:p.cursor-1]
	tokensAfter := p.tokens[p.cursor+2:]

	// splice the imported tokens in the place of the import statement
	// and rewind cursor so Next() will land on first imported token
	p.tokens = append(tokensBefore, append(importedTokens, tokensAfter...)...)
	p.cursor--
	return nil
}

// doSingleImport lexes the individual file at importFile and returns
// its tokens or an error, if any.
func (p *nginxParser) doSingleInclude(importFile string) ([]token, error) {
	file, err := os.Open(importFile)
	if err != nil {
		return nil, fmt.Errorf("Could not import %s: %v", importFile, err)
	}
	defer file.Close()

	if info, err := file.Stat(); err != nil {
		return nil, fmt.Errorf("Could not import %s: %v", importFile, err)
	} else if info.IsDir() {
		return nil, fmt.Errorf("Could not import %s: is a directory", importFile)
	}

	input, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("Could not read imported file %s: %v", importFile, err)
	}
	importedTokens := allTokens(importFile, input)
	return importedTokens, nil
}

// allTokens lexes the entire input, but does not parse it.
// It returns all the tokens from the input, unstructured
// and in order.
func allTokens(filename string, input []byte) []token {
	tokens := tokenize(input)
	for i := range tokens {
		tokens[i].file = filename
	}
	return tokens
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

//+build ignore

package main

import (
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type Directive struct {
	Name     string   `xml:"name,attr"`
	Contexts []string `xml:"context"`
}

type Module struct {
	Section []struct {
		Directive []Directive `xml:"directive"`
	} `xml:"section"`
}

const pathToDocs = "/xml/en/docs/http/ngx_*"

func main() {
	if len(os.Args) < 2 {
		log.Println("provide the path to the directory of nginx.org repo as argument")
		os.Exit(1)
	}
	fullPath := filepath.Clean(filepath.Join(os.Args[1], pathToDocs))
	matches, err := filepath.Glob(fullPath)
	if err != nil {
		log.Printf("error globing: %s", err)
		os.Exit(1)
	}
	contextDirs := make(map[string][]string)
	for _, v := range matches {
		mods := []Module{}
		func(m string) {
			f, err := os.Open(m)
			if err != nil {
				log.Printf("error openning the file %s : %s", m, err)
				return
			}
			defer f.Close()
			dec := xml.NewDecoder(f)
			dec.Strict = false // "Strict" implies it doesn't recognize &nbsp and &mdash

			err = dec.Decode(&mods)
			if err != nil {
				// It complains about unexpected EOF in ngx_http_api_module_head.xml on line 261, which is actually the end of file.
				// So ¯\_(ツ)_/¯
				log.Printf("error unmarshalling the file %s : %s", m, err)
			}
		}(v)
		for _, m := range mods {
			if len(m.Section) == 0 {
				continue
			}
			for _, s := range m.Section {
				if len(s.Directive) == 0 {
					continue
				}
				for _, d := range s.Directive {
					for _, c := range d.Contexts {
						contextDirs[c] = append(contextDirs[c], d.Name)
					}
				}
			}
		}
	}

	for ctx, dirs := range contextDirs {
		func(ctx string, dirs []string) {
			ctx = strings.ReplaceAll(ctx, " ", "_")
			f, err := os.Create(fmt.Sprintf("%s.txt", ctx))
			if err != nil {
				log.Printf("Error creating out file: %s", err)
				return
			}
			defer f.Close()
			for _, d := range dirs {
				f.WriteString(fmt.Sprintf("%s\n", d))
			}
		}(ctx, dirs)
	}
}

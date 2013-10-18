/*
 * Copyright (C) 2012 Chandra Sekar S
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

func main() {
	addr := flag.String("a", ":9090", "address to listen on (host:port)")
	masqHost := flag.String("m", "", "pretend to be listening on (host)")
	pkgFile := flag.String("p", "", "package list")
	help := flag.Bool("help", false, "print usage")

	flag.Parse()
	if *help {
		flag.Usage()
		return
	}

	if *pkgFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	pl, err := NewPackageList(*pkgFile, *masqHost)
	if err != nil {
		log.Fatalln("Reading package list failed:", err)
	}

	fmt.Printf("Serving package(s) on %s...\n", *addr)
	err = http.ListenAndServe(*addr, pl)
	if err != nil {
		log.Fatalln("Server failed to start:", err)
	}
}

type PackageList struct {
	packages map[string]*Package
	mx       sync.RWMutex
	file     string
	masqHost string
}

func NewPackageList(pkgFile string, masqHost string) (pl *PackageList, err error) {
	pl = &PackageList{file: pkgFile, masqHost: masqHost}

	err = pl.loadPackages()
	if err != nil {
		return nil, err
	}

	return pl, nil
}

func (pl *PackageList) loadPackages() error {
	pl.mx.Lock()
	defer pl.mx.Unlock()

	f, err := os.Open(pl.file)
	if err != nil {
		return err
	}
	defer f.Close()
	in := bufio.NewReader(f)

	pkgs := make(map[string]*Package)
	for {
		ln, _, err := in.ReadLine()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		line := string(ln)
		if len(strings.TrimSpace(line)) > 0 {
			pkg := NewPackage(line)
			pkgs[pkg.Path] = pkg
		}
	}
	pl.packages = pkgs

	return nil
}

func (pl *PackageList) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pl.mx.RLock()
	defer pl.mx.RUnlock()

	host := pl.masqHost
	if host == "" {
		host = r.Host
	}

	if r.URL.Path == "/" {
		indexTmpl.Execute(w, map[string]interface{}{
			"host": host,
			"pkgs": pl.packages,
		})
	} else {
		if pkg, ok := pl.getPackage(r.URL.Path); ok {
			if r.FormValue("go-get") == "1" || pkg.Doc == "" {
				pkgTmpl.Execute(w, map[string]interface{}{
					"host": host,
					"pkg":  pkg,
				})
			} else {
				http.Redirect(w, r, pkg.Doc, http.StatusFound)
			}
		} else {
			http.NotFound(w, r)
		}
	}
}

func (pl *PackageList) getPackage(path string) (*Package, bool) {
	if pkg, ok := pl.packages[path]; ok {
		return pkg, ok
	}

	for prefix := path; prefix != ""; prefix = prefix[:strings.LastIndex(prefix, "/")] {
		if pkg, ok := pl.packages[prefix]; ok {
			return pkg, ok
		}
	}

	return nil, false
}

type Package struct {
	Path, Vcs, Repo, Doc string
}

func NewPackage(line string) *Package {
	fields := strings.Fields(line)
	doc := ""
	if len(fields) > 3 {
		doc = fields[3]
	}

	return &Package{fields[0], fields[1], fields[2], doc}
}

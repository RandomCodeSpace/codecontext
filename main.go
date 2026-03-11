package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var version = "dev"

const usage = `codecontext - aggregate source files into a single context block

Usage:
  codecontext [flags] [files/dirs...]

Flags:
  -ext string    Comma-separated list of file extensions to include (e.g. .go,.ts)
  -version       Print version and exit
  -help          Print this help message

Examples:
  codecontext .                        # all files in current directory
  codecontext -ext .go,.md .           # only .go and .md files
  codecontext main.go go.mod README.md # specific files
`

func main() {
	ext := flag.String("ext", "", "comma-separated file extensions to include")
	ver := flag.Bool("version", false, "print version")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	if *ver {
		fmt.Printf("codecontext %s\n", version)
		return
	}

	args := flag.Args()
	if len(args) == 0 {
		args = []string{"."}
	}

	var exts map[string]bool
	if *ext != "" {
		exts = make(map[string]bool)
		for _, e := range strings.Split(*ext, ",") {
			e = strings.TrimSpace(e)
			if !strings.HasPrefix(e, ".") {
				e = "." + e
			}
			exts[e] = true
		}
	}

	var files []string
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if info.IsDir() {
			walked, err := walkDir(arg, exts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			files = append(files, walked...)
		} else {
			files = append(files, arg)
		}
	}

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "no files found")
		os.Exit(1)
	}

	for _, f := range files {
		if err := printFile(f); err != nil {
			fmt.Fprintf(os.Stderr, "error reading %s: %v\n", f, err)
		}
	}
}

func walkDir(dir string, exts map[string]bool) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if exts != nil && !exts[filepath.Ext(path)] {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func printFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fmt.Printf("=== %s ===\n%s\n", path, string(data))
	return nil
}

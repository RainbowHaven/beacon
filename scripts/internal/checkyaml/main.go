// Command checkyaml exits 0 if each argument is valid YAML.
package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: checkyaml <file>...")
		os.Exit(2)
	}
	for _, path := range os.Args[1:] {
		b, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
		var v any
		if err := yaml.Unmarshal(b, &v); err != nil {
			fmt.Fprintf(os.Stderr, "%s: invalid YAML: %v\n", path, err)
			os.Exit(1)
		}
	}
}

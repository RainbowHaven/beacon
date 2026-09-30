//go:build ignore

// gen_aaguids refreshes aaguids.json from the community AAGUID list, keeping
// only the authenticator names. Run with `go generate ./internal/passkeylabel`.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const source = "https://raw.githubusercontent.com/passkeydeveloper/passkey-authenticator-aaguids/main/combined_aaguid.json"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen_aaguids:", err)
		os.Exit(1)
	}
}

func run() error {
	res, err := http.Get(source)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", source, res.Status)
	}
	var list map[string]struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		return err
	}
	names := make(map[string]string, len(list))
	for id, v := range list {
		if name := strings.Join(strings.Fields(v.Name), " "); name != "" {
			names[strings.ToLower(id)] = name
		}
	}
	out, err := json.MarshalIndent(map[string]any{
		"source":    source,
		"retrieved": time.Now().UTC().Format("2006-01-02"),
		"names":     names,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("aaguids.json", append(out, '\n'), 0o644)
}

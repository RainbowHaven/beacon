// Package passkeylabel names a passkey after the device, OS version and
// passkey provider it was created with, e.g. "iPhone · iOS 18 · Apple Passwords".
package passkeylabel

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

//go:generate go run gen_aaguids.go

//go:embed aaguids.json
var aaguidJSON []byte

var providers = func() map[string]string {
	var f struct {
		Names map[string]string `json:"names"`
	}
	if err := json.Unmarshal(aaguidJSON, &f); err != nil {
		panic("passkeylabel: aaguids.json: " + err.Error())
	}
	return f.Names
}()

const (
	sep      = " · "
	maxModel = 40
)

// Input is everything known about a passkey when it is registered. Stored
// credentials only have AAGUID and Transports.
type Input struct {
	UserAgent       string
	Platform        string // Sec-CH-UA-Platform
	PlatformVersion string // Sec-CH-UA-Platform-Version
	Model           string // Sec-CH-UA-Model
	Mobile          string // Sec-CH-UA-Mobile
	AAGUID          []byte
	Transports      []string
	Attachment      string // authenticatorAttachment of the registration response
}

// FromHeader reads the User-Agent and the User-Agent Client Hints of r.
func FromHeader(h http.Header) Input {
	return Input{
		UserAgent:       h.Get("User-Agent"),
		Platform:        h.Get("Sec-CH-UA-Platform"),
		PlatformVersion: h.Get("Sec-CH-UA-Platform-Version"),
		Model:           h.Get("Sec-CH-UA-Model"),
		Mobile:          h.Get("Sec-CH-UA-Mobile"),
	}
}

// Derive returns "Device · OS Version · Provider", leaving out any part that
// cannot be determined. Security keys are "Key name · security key".
func Derive(in Input) string {
	provider := Provider(in.AAGUID)
	if in.securityKey() {
		if provider == "" {
			return "Security key"
		}
		return provider + sep + "security key"
	}
	var device, os string
	switch {
	case in.Attachment == "cross-platform" && in.has("hybrid"):
		device = "Phone or tablet"
	case in.Attachment == "cross-platform":
	default:
		device, os = in.device()
	}
	var parts []string
	for _, p := range []string{device, os, provider} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return "Passkey"
	}
	return strings.Join(parts, sep)
}

// Provider is the authenticator name for aaguid, or "" when it is all zeros
// or not in the list.
func Provider(aaguid []byte) string {
	if len(aaguid) != 16 || !slices.ContainsFunc(aaguid, func(b byte) bool { return b != 0 }) {
		return ""
	}
	b := aaguid
	return providers[fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])]
}

func (in Input) has(transport string) bool {
	return slices.Contains(in.Transports, transport)
}

func (in Input) securityKey() bool {
	roaming := in.has("usb") || in.has("nfc") || in.has("ble") || in.has("smart-card")
	return roaming && !in.has("internal") && !in.has("hybrid")
}

var (
	appleOSRe       = regexp.MustCompile(`OS (\d+)_(\d+)`)
	safariVersionRe = regexp.MustCompile(`Version/(\d+)`)
	androidRe       = regexp.MustCompile(`Android (\d+)(?:\.[\d.]*)?(?:; ([^;)]+))?`)
)

func (in Input) device() (device, os string) {
	ua := in.UserAgent
	platform := unquote(in.Platform)
	version, hasVersion := major(unquote(in.PlatformVersion))
	switch {
	case strings.Contains(ua, "iPhone"):
		return "iPhone", appleMobileOS("iOS", ua)
	case strings.Contains(ua, "iPad"):
		return "iPad", appleMobileOS("iPadOS", ua)
	case strings.Contains(ua, "Android") || platform == "Android":
		return in.android(platform == "Android" && hasVersion && version > 0, version)
	case strings.Contains(ua, "CrOS") || platform == "Chrome OS":
		// The Chrome OS platform version is a build number, not the release.
		return "Chromebook", "ChromeOS"
	case strings.Contains(ua, "Macintosh") || platform == "macOS":
		// macOS UAs are frozen at 10_15_7 and iPads in desktop mode send the
		// same UA, so only a Client Hint version is trusted.
		if platform == "macOS" && hasVersion && version > 0 {
			return "Mac", fmt.Sprintf("macOS %d", version)
		}
		return "Mac", "macOS"
	case strings.Contains(ua, "Windows") || platform == "Windows":
		// The UA is frozen at "Windows NT 10.0"; the hint tells 10 from 11.
		switch {
		case platform != "Windows" || !hasVersion || version == 0:
			return "Windows PC", "Windows"
		case version >= 13:
			return "Windows PC", "Windows 11"
		default:
			return "Windows PC", "Windows 10"
		}
	case strings.Contains(ua, "Linux") || platform == "Linux":
		return "Linux PC", "Linux"
	}
	return "", ""
}

// appleMobileOS reads the iOS/iPadOS version from the UA. From release 26
// WebKit freezes the OS token at 18_6 and Safari reports the release as
// Version/26, so a bare 18_6 without such a Version is ambiguous.
func appleMobileOS(name, ua string) string {
	if m := safariVersionRe.FindStringSubmatch(ua); m != nil {
		if v, _ := strconv.Atoi(m[1]); v >= 26 {
			return fmt.Sprintf("%s %d", name, v)
		}
	}
	m := appleOSRe.FindStringSubmatch(ua)
	if m == nil || (m[1] == "18" && m[2] == "6" && !safariVersionRe.MatchString(ua)) {
		return name
	}
	return name + " " + m[1]
}

// android handles Chrome's reduced UA "Android 10; K", which carries neither
// the real version nor the model.
func (in Input) android(hintVersion bool, version int) (device, os string) {
	ua := in.UserAgent
	m := androidRe.FindStringSubmatch(ua)
	reduced := m != nil && m[1] == "10" && m[2] == "K"

	device = cleanModel(unquote(in.Model))
	if device == "" && m != nil && !reduced {
		model, _, _ := strings.Cut(m[2], " Build/")
		if model != "Mobile" && model != "Tablet" && model != "wv" {
			device = cleanModel(model)
		}
	}
	if device == "" {
		if in.Mobile == "?1" || strings.Contains(ua, "Mobile") {
			device = "Android phone"
		} else {
			device = "Android tablet"
		}
	}

	switch {
	case hintVersion:
		os = fmt.Sprintf("Android %d", version)
	case m != nil && !reduced:
		os = "Android " + m[1]
	default:
		os = "Android"
	}
	return device, os
}

func major(v string) (int, bool) {
	head, _, _ := strings.Cut(v, ".")
	n, err := strconv.Atoi(head)
	return n, err == nil
}

func unquote(s string) string {
	return strings.Trim(strings.TrimSpace(s), `"`)
}

// cleanModel keeps a client-supplied model name printable and short.
func cleanModel(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxModel {
		s = strings.TrimSpace(string(r[:maxModel]))
	}
	return s
}

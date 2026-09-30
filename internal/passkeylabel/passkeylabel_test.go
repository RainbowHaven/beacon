package passkeylabel

import (
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
)

const (
	applePasswords = "fbfc3007-154e-4ecc-8c0b-6e020557d7bd"
	chromeOnMac    = "adce0002-35bc-c60a-648b-0b25f1f05503"
	windowsHello   = "08987058-cadc-4b81-b6e1-30de50dcbe96"
	googlePM       = "ea9b8d66-4d01-1d21-3ce4-b6b48cb575d4"
	bitwarden      = "d548826e-79b4-db40-a3d8-11116f7e8349"
	samsungPass    = "53414d53-554e-4700-0000-000000000000"
	yubiKey5NFC    = "fa2b99dc-9e39-4257-8f92-4a30d23c4118"
	yubiKey5       = "ee882879-721c-4913-9775-3dfcce97072a"
	zeroAAGUID     = "00000000-0000-0000-0000-000000000000"
	unknownAAGUID  = "01234567-89ab-cdef-0123-456789abcdef"

	uaIPhoneSafari18 = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.2 Mobile/15E148 Safari/604.1"
	uaIPhoneSafari26 = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Mobile/15E148 Safari/604.1"
	uaIPhoneChrome26 = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/140.0.7339.101 Mobile/15E148 Safari/604.1"
	uaIPad           = "Mozilla/5.0 (iPad; CPU OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1"
	uaMacSafari      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15"
	uaMacChrome      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	uaWindowsChrome  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	uaWindowsFirefox = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0"
	uaAndroidChrome  = "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36"
	uaAndroidTablet  = "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	uaAndroidFirefox = "Mozilla/5.0 (Android 14; Mobile; rv:133.0) Gecko/133.0 Firefox/133.0"
	uaSamsung        = "Mozilla/5.0 (Linux; Android 13; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/23.0 Chrome/115.0.0.0 Mobile Safari/537.36"
	uaLinuxFirefox   = "Mozilla/5.0 (X11; Linux x86_64; rv:133.0) Gecko/20100101 Firefox/133.0"
	uaChromeOS       = "Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

func aaguid(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	if err != nil || len(b) != 16 {
		t.Fatalf("bad aaguid %q", s)
	}
	return b
}

// Rules, in order:
//   - usb/nfc/ble/smart-card transports without internal/hybrid: "Key name · security key"
//     ("Security key" when the AAGUID is unknown); the browser's device is not the key.
//   - cross-platform with hybrid (passkey on a phone via QR code): "Phone or tablet · Provider".
//   - otherwise "Device · OS Version · Provider" from the UA and Client Hints.
//   - a part that cannot be determined is left out; nothing known at all is "Passkey".
//   - OS versions: iOS/iPadOS from the UA (Version/26+ from release 26, bare 18_6 is
//     ambiguous); macOS only from the platform version hint; Windows 11 when the hint
//     major is >= 13, Windows 10 for 1-12; Android from the hint, or from an unreduced UA.
func TestDerive(t *testing.T) {
	internal := []string{"hybrid", "internal"}
	tests := []struct {
		name string
		in   Input
		id   string
		want string
	}{
		{"iPhone Safari", Input{UserAgent: uaIPhoneSafari18, Transports: internal}, applePasswords, "iPhone · iOS 18 · Apple Passwords"},
		{"iPhone Safari 26 frozen OS token", Input{UserAgent: uaIPhoneSafari26, Transports: internal}, applePasswords, "iPhone · iOS 26 · Apple Passwords"},
		{"iPhone Chrome 26 ambiguous", Input{UserAgent: uaIPhoneChrome26, Transports: internal}, applePasswords, "iPhone · iOS · Apple Passwords"},
		{"iPad Safari", Input{UserAgent: uaIPad, Transports: internal}, applePasswords, "iPad · iPadOS 17 · Apple Passwords"},
		{"Mac Safari hides version", Input{UserAgent: uaMacSafari, Transports: internal}, applePasswords, "Mac · macOS · Apple Passwords"},
		{"Mac Chrome with hints", Input{UserAgent: uaMacChrome, Platform: `"macOS"`, PlatformVersion: `"15.1.0"`, Mobile: "?0", Transports: internal}, applePasswords, "Mac · macOS 15 · Apple Passwords"},
		{"Mac Chrome profile", Input{UserAgent: uaMacChrome, Platform: `"macOS"`, PlatformVersion: `"26.0.0"`, Transports: []string{"internal"}}, chromeOnMac, "Mac · macOS 26 · Chrome on Mac"},
		{"Mac Chrome without high-entropy hints", Input{UserAgent: uaMacChrome, Platform: `"macOS"`, Transports: internal}, bitwarden, "Mac · macOS · Bitwarden"},
		{"Windows 11 Chrome", Input{UserAgent: uaWindowsChrome, Platform: `"Windows"`, PlatformVersion: `"15.0.0"`, Transports: []string{"internal"}}, windowsHello, "Windows PC · Windows 11 · Windows Hello"},
		{"Windows 11 first hint major", Input{UserAgent: uaWindowsChrome, Platform: `"Windows"`, PlatformVersion: `"13.0.0"`, Transports: []string{"internal"}}, windowsHello, "Windows PC · Windows 11 · Windows Hello"},
		{"Windows 10 Chrome", Input{UserAgent: uaWindowsChrome, Platform: `"Windows"`, PlatformVersion: `"10.0.0"`, Transports: []string{"internal"}}, windowsHello, "Windows PC · Windows 10 · Windows Hello"},
		{"Windows 8.1 Chrome", Input{UserAgent: uaWindowsChrome, Platform: `"Windows"`, PlatformVersion: `"0.3.0"`, Transports: []string{"internal"}}, windowsHello, "Windows PC · Windows · Windows Hello"},
		{"Windows Firefox", Input{UserAgent: uaWindowsFirefox, Transports: []string{"internal"}}, windowsHello, "Windows PC · Windows · Windows Hello"},
		{"Android Chrome with hints", Input{UserAgent: uaAndroidChrome, Platform: `"Android"`, PlatformVersion: `"15.0.0"`, Model: `"Pixel 8"`, Mobile: "?1", Transports: internal}, googlePM, "Pixel 8 · Android 15 · Google Password Manager"},
		{"Android Chrome reduced UA only", Input{UserAgent: uaAndroidChrome, Platform: `"Android"`, Model: `""`, Mobile: "?1", Transports: internal}, googlePM, "Android phone · Android · Google Password Manager"},
		{"Android tablet reduced UA", Input{UserAgent: uaAndroidTablet, Platform: `"Android"`, Mobile: "?0", Transports: internal}, googlePM, "Android tablet · Android · Google Password Manager"},
		{"Android Firefox", Input{UserAgent: uaAndroidFirefox, Transports: internal}, googlePM, "Android phone · Android 14 · Google Password Manager"},
		{"Samsung Internet unreduced UA", Input{UserAgent: uaSamsung, Transports: internal}, samsungPass, "SM-S918B · Android 13 · Samsung Pass"},
		{"Linux Firefox", Input{UserAgent: uaLinuxFirefox, Transports: internal}, bitwarden, "Linux PC · Linux · Bitwarden"},
		{"ChromeOS", Input{UserAgent: uaChromeOS, Platform: `"Chrome OS"`, PlatformVersion: `"14541.0.0"`, Transports: internal}, googlePM, "Chromebook · ChromeOS · Google Password Manager"},
		{"security key over USB", Input{UserAgent: uaMacChrome, Platform: `"macOS"`, PlatformVersion: `"15.1.0"`, Transports: []string{"nfc", "usb"}, Attachment: "cross-platform"}, yubiKey5NFC, "YubiKey 5 Series with NFC · security key"},
		{"security key over NFC on iPhone", Input{UserAgent: uaIPhoneSafari18, Transports: []string{"nfc"}}, yubiKey5, "YubiKey 5 Series · security key"},
		{"unknown security key", Input{UserAgent: uaWindowsFirefox, Transports: []string{"usb"}}, zeroAAGUID, "Security key"},
		{"phone via QR code", Input{UserAgent: uaWindowsChrome, Platform: `"Windows"`, PlatformVersion: `"15.0.0"`, Transports: internal, Attachment: "cross-platform"}, googlePM, "Phone or tablet · Google Password Manager"},
		{"cross-platform without transports", Input{UserAgent: uaWindowsChrome, Attachment: "cross-platform"}, yubiKey5, "YubiKey 5 Series"},
		{"platform without provider", Input{UserAgent: uaIPhoneSafari18, Transports: internal}, zeroAAGUID, "iPhone · iOS 18"},
		{"unknown AAGUID", Input{UserAgent: uaLinuxFirefox}, unknownAAGUID, "Linux PC · Linux"},
		{"stored credential with provider", Input{}, applePasswords, "Apple Passwords"},
		{"stored security key", Input{Transports: []string{"usb"}}, yubiKey5, "YubiKey 5 Series · security key"},
		{"stored credential, nothing known", Input{Transports: []string{"internal"}}, zeroAAGUID, "Passkey"},
		{"no AAGUID", Input{}, "", "Passkey"},
		{"unknown browser", Input{UserAgent: "curl/8.7.1"}, googlePM, "Google Password Manager"},
		{"model hint is sanitized", Input{UserAgent: uaAndroidChrome, Platform: `"Android"`, PlatformVersion: `"14"`, Model: "\"  Galaxy\tTab\x00S9 \"", Transports: internal}, zeroAAGUID, "Galaxy Tab S9 · Android 14"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in
			if tt.id != "" {
				in.AAGUID = aaguid(t, tt.id)
			}
			got := Derive(in)
			if got != tt.want {
				t.Fatalf("Derive = %q, want %q", got, tt.want)
			}
			if again := Derive(in); again != got {
				t.Fatalf("Derive not stable: %q then %q", got, again)
			}
		})
	}
}

func TestProvider(t *testing.T) {
	if len(providers) < 100 {
		t.Fatalf("embedded AAGUID list has only %d entries", len(providers))
	}
	for id, name := range providers {
		if id != strings.ToLower(id) || strings.TrimSpace(name) != name || name == "" {
			t.Fatalf("bad entry %q: %q", id, name)
		}
	}
	if got := Provider(aaguid(t, googlePM)); got != "Google Password Manager" {
		t.Fatalf("Provider = %q", got)
	}
	for _, b := range [][]byte{nil, {1, 2, 3}, make([]byte, 16), aaguid(t, unknownAAGUID)} {
		if got := Provider(b); got != "" {
			t.Fatalf("Provider(%x) = %q, want empty", b, got)
		}
	}
}

func TestFromHeader(t *testing.T) {
	h := http.Header{}
	h.Set("User-Agent", uaAndroidChrome)
	h.Set("Sec-CH-UA-Platform", `"Android"`)
	h.Set("Sec-CH-UA-Platform-Version", `"15.0.0"`)
	h.Set("Sec-CH-UA-Model", `"Pixel 8"`)
	h.Set("Sec-CH-UA-Mobile", "?1")
	in := FromHeader(h)
	in.AAGUID = aaguid(t, googlePM)
	if got := Derive(in); got != "Pixel 8 · Android 15 · Google Password Manager" {
		t.Fatalf("Derive(FromHeader) = %q", got)
	}
}

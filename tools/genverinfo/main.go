// Command genverinfo renders build/windows/versioninfo.json.tmpl into a
// temporary versioninfo JSON carrying the per-architecture identity, so the
// built executable's Properties -> Details always shows the platform it was
// built for (e.g. FileDescription "WireGuide Plus (amd64) - ...", InternalName
// "wireguideplus-amd64.exe").
//
//	genverinfo -arch amd64 -version 1.1.1 \
//	          -tmpl windows/versioninfo.json.tmpl \
//	          -out  windows/versioninfo.gen.json
//
// Stdlib-only so CI needs nothing beyond the Go toolchain, mirroring
// tools/updatesign.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/template"
)

type info struct {
	Major, Minor, Build, Patch int
	Version                     string
	Suffix                      string
	Description                 string
	Executable                  string
	ProductName                 string
	CompanyName                 string
	Copyright                   string
	Comments                    string
}

func main() {
	arch := flag.String("arch", "amd64", "GOARCH of the build (386/amd64/arm64)")
	version := flag.String("version", "1.1.1", "product version as x.y.z")
	tmplPath := flag.String("tmpl", "windows/versioninfo.json.tmpl", "path to the template, relative to CWD")
	outPath := flag.String("out", "windows/versioninfo.gen.json", "path to write the rendered JSON")
	flag.Parse()

	suffix := *arch
	if suffix == "386" {
		suffix = "x86"
	}

	v, err := parseVersion(*version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "genverinfo:", err)
		os.Exit(1)
	}

	data := info{
		Major:       v[0],
		Minor:       v[1],
		Build:       v[2],
		Patch:       v[3],
		Version:     *version,
		Suffix:      suffix,
		Description: fmt.Sprintf("WireGuide Plus (%s) - multi-tunnel automated WireGuard VPN client", suffix),
		Executable:  fmt.Sprintf("wireguideplus-%s.exe", suffix),
		ProductName: "WireGuide Plus",
		CompanyName: "imonior",
		Copyright:   "\u00A9 2026 imonior",
		Comments:    "https://github.com/imonior/wireguide-plus",
	}

	tmpl, err := template.ParseFiles(*tmplPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "genverinfo:", err)
		os.Exit(1)
	}
	out, err := os.Create(*outPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "genverinfo:", err)
		os.Exit(1)
	}
	defer out.Close()
	if err := tmpl.Execute(out, data); err != nil {
		fmt.Fprintln(os.Stderr, "genverinfo:", err)
		os.Exit(1)
	}
}

func parseVersion(s string) ([4]int, error) {
	var v [4]int
	parts := strings.Split(s, ".")
	if len(parts) > 4 {
		return v, fmt.Errorf("invalid version %q", s)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return v, fmt.Errorf("invalid version %q", s)
		}
		v[i] = n
	}
	return v, nil
}

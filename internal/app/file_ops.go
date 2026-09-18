package app

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg" // QR codes may arrive as JPEG (phone-camera capture, exports)
	_ "image/png"  // ...or PNG (most common — qrencode + WG mobile apps default)
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imonior/wireguide-plus/internal/config"
	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
	_ "golang.org/x/image/webp" // ...or WebP (Safari/Chrome image caches, modern web exports)
)

// ZipImportResult holds the outcome of importing one .conf entry from a zip.
type ZipImportResult struct {
	Name  string `json:"name"`
	Error string `json:"error,omitempty"`
}

// zipUniqueName returns a tunnel name that doesn't conflict with existing ones.
func (s *TunnelService) zipUniqueName(base string) string {
	if !s.tunnelStore.Exists(base) {
		return base
	}
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !s.tunnelStore.Exists(candidate) {
			return candidate
		}
	}
	return fmt.Sprintf("%s-%d", base, time.Now().UnixMilli())
}

// ImportZip extracts all .conf files from a zip archive and imports each one.
// Returns per-file results; an error is only returned for zip-level failures.
func (s *TunnelService) ImportZip(path string) ([]ZipImportResult, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("opening zip: %w", err)
	}
	defer r.Close()
	return s.importZipReader(&r.Reader)
}

// maxZipDataSize caps the in-memory zip payload accepted from the GUI
// file picker. WireGuard configs are tiny; a multi-MB zip dump is
// either a mistake or a memory-exhaustion attempt.
const maxZipDataSize = 32 << 20

// maxZipEntrySize bounds each decompressed entry (zip-bomb guard).
const maxZipEntrySize = 1 << 20

// ImportZipData imports a zip supplied as raw bytes (used by the file picker,
// which provides a File object rather than a filesystem path).
func (s *TunnelService) ImportZipData(data []byte) ([]ZipImportResult, error) {
	if len(data) > maxZipDataSize {
		slog.Warn("tunnel: zip import failed (too large)", "category", "tunnel", "bytes", len(data), "max", maxZipDataSize)
		return nil, fmt.Errorf("zip too large: %d bytes (max %d)", len(data), maxZipDataSize)
	}
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		slog.Warn("tunnel: zip import failed (read)", "category", "tunnel", "bytes", len(data), "error", err)
		return nil, fmt.Errorf("reading zip: %w", err)
	}
	return s.importZipReader(r)
}

// importZipReader is the shared implementation for ImportZip and ImportZipData.
func (s *TunnelService) importZipReader(r *zip.Reader) ([]ZipImportResult, error) {
	var results []ZipImportResult
	for _, f := range r.File {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".conf") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			results = append(results, ZipImportResult{Name: filepath.Base(f.Name), Error: err.Error()})
			continue
		}
		// Per-entry size cap — zip bomb protection.
		data, err := io.ReadAll(io.LimitReader(rc, maxZipEntrySize+1))
		rc.Close()
		if err != nil {
			results = append(results, ZipImportResult{Name: filepath.Base(f.Name), Error: err.Error()})
			continue
		}
		if int64(len(data)) > maxZipEntrySize {
			results = append(results, ZipImportResult{
				Name:  filepath.Base(f.Name),
				Error: fmt.Sprintf("entry exceeds %d bytes", maxZipEntrySize),
			})
			continue
		}
		baseName := strings.TrimSuffix(filepath.Base(f.Name), ".conf")
		name := s.zipUniqueName(baseName)
		if _, err := s.ImportConfig(name, string(data)); err != nil {
			results = append(results, ZipImportResult{Name: baseName, Error: err.Error()})
		} else {
			results = append(results, ZipImportResult{Name: name})
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no .conf files found in zip")
	}
	ok, failed := 0, 0
	for _, r := range results {
		if r.Error == "" {
			ok++
		} else {
			failed++
		}
	}
	slog.Info("tunnel: zip imported", "category", "tunnel", "imported", ok, "failed", failed)
	return results, nil
}

// ImportConfig parses, validates, and saves a tunnel config under the given
// name. Returns a TunnelInfo for optimistic UI display.
func (s *TunnelService) ImportConfig(name, content string) (*TunnelInfo, error) {
	cfg, err := s.tunnelStore.ImportFromContent(name, content)
	if err != nil {
		slog.Warn("tunnel: import failed", "category", "tunnel", "tunnel", name, "error", err)
		return nil, err
	}
	endpoint := ""
	if len(cfg.Peers) > 0 {
		endpoint = cfg.Peers[0].Endpoint
	}
	slog.Info("tunnel: imported", "category", "tunnel", "tunnel", cfg.Name, "endpoint", endpoint)
	return &TunnelInfo{
		Name:     cfg.Name,
		Endpoint: endpoint,
	}, nil
}

// maxReadFileSize is the largest file ReadFile will accept (10 MB).
// WireGuard configs are typically a few KB; anything larger is almost
// certainly not a valid .conf file.
const maxReadFileSize = 10 << 20

// maxQRImageSize bounds the image we'll try to decode. QR images shared by
// VPN providers are typically under 200 KB; the cap protects against an
// accidentally-supplied multi-MB photo or a hostile oversize image.
const maxQRImageSize = 8 << 20

// decodeQRConfig decodes a QR code from raw image bytes and returns its
// payload. Returns a stable error message ("no WireGuard QR code...") on any
// decode failure so the frontend can show a single, consistent message
// regardless of whether the issue was an unsupported image format, a missing
// QR code, or a QR code without a [Interface] section.
func decodeQRConfig(data []byte) (string, error) {
	if len(data) > maxQRImageSize {
		return "", fmt.Errorf("image too large (%d bytes, max %d)", len(data), maxQRImageSize)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("cannot decode image: %w", err)
	}
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", fmt.Errorf("no WireGuard QR code found in image")
	}
	res, err := qrcode.NewQRCodeReader().Decode(bmp, nil)
	if err != nil {
		return "", fmt.Errorf("no WireGuard QR code found in image")
	}
	text := res.GetText()
	if !strings.Contains(text, "[Interface]") {
		return "", fmt.Errorf("QR code does not contain a WireGuard config")
	}
	return text, nil
}

// ImportQRFromPath reads an image from disk, decodes its QR code, and imports
// the contained WireGuard config under the given name.
func (s *TunnelService) ImportQRFromPath(path, name string) (*TunnelInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if info.Size() > maxQRImageSize {
		return nil, fmt.Errorf("file too large (%d bytes, max %d)", info.Size(), maxQRImageSize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return s.ImportQRFromBytes(data, name)
}

// ImportQRFromBytes decodes a QR code from raw image bytes (typically supplied
// by the file picker, which gives us bytes rather than a path) and imports the
// resulting WireGuard config.
func (s *TunnelService) ImportQRFromBytes(data []byte, name string) (*TunnelInfo, error) {
	text, err := decodeQRConfig(data)
	if err != nil {
		return nil, err
	}
	return s.ImportConfig(name, text)
}

// ReadFile reads a file from disk (used by native file drop). Returns the
// content as a string so the frontend can handle name conflicts before import.
func (s *TunnelService) ReadFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		slog.Warn("tunnel: read file failed (stat)", "category", "tunnel", "path", path, "error", err)
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	if info.Size() > maxReadFileSize {
		slog.Warn("tunnel: read file failed (too large)", "category", "tunnel", "path", path, "bytes", info.Size(), "max", maxReadFileSize)
		return "", fmt.Errorf("file %s is too large (%d bytes, max %d)", path, info.Size(), maxReadFileSize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		slog.Warn("tunnel: read file failed (read)", "category", "tunnel", "path", path, "error", err)
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	return string(data), nil
}

// BaseName extracts the filename without extension from a path.
func (s *TunnelService) BaseName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

// ValidateConfig parses and validates a raw config string. Returns a list of
// human-readable error messages, or nil if the config is valid.
func (s *TunnelService) ValidateConfig(content string) ([]string, error) {
	cfg, err := config.Parse(content)
	if err != nil {
		// Logged on purpose: this is the shared front door for both "save an
		// edited tunnel" and "import a .conf", and a parse failure here used
		// to be reported to the GUI with no server-side trace at all, which
		// made a rejected save impossible to diagnose from the logs.
		slog.Warn("tunnel: validate failed (parse)", "category", "tunnel", "error", err)
		return []string{err.Error()}, nil
	}
	result := config.Validate(cfg)
	if result.IsValid() {
		return nil, nil
	}
	msgs := result.ErrorMessages()
	slog.Warn("tunnel: validate failed (validation)", "category", "tunnel", "errors", strings.Join(msgs, "; "))
	return msgs, nil
}

// GetConfigText returns the serialized form of a stored tunnel's config.
func (s *TunnelService) GetConfigText(name string) (string, error) {
	cfg, err := s.tunnelStore.Load(name)
	if err != nil {
		slog.Warn("tunnel: load config text failed", "category", "tunnel", "tunnel", name, "error", err)
		return "", err
	}
	return config.Serialize(cfg), nil
}

// UpdateConfig parses, validates, and overwrites an existing tunnel's config.
//
// Editing a CONNECTED tunnel is allowed. It used to be rejected outright, and
// that refusal is exactly what made saving impossible: the GUI could not
// translate the English error into anything actionable, so Save looked dead
// unless you already knew the tunnel had to be stopped first.
//
// Editing != applying. The running instance keeps using the configuration it
// was handed at connect time, so whoever changes the file owns the restart:
// stop the tunnel, save, then reconnect it. persistEditorSave does this for
// the GUI. The price of getting it wrong is a tunnel that runs the previous
// config until the next connect — not corruption — which is why this is a
// logged warning rather than an error.
func (s *TunnelService) UpdateConfig(name, content string) error {
	// The state probe below only decides WHICH log line we write — it must
	// never block the write itself. It used to return an error on any IPC
	// hiccup, and a transient helper problem then made every edit fail with
	// an English "cannot verify tunnel state" that the GUI could only echo
	// verbatim and that left no trace in the log, so "helper slow for one
	// call" was indistinguishable from "editing is broken".
	if active, stateErr := s.isActiveTunnel(name); stateErr != nil {
		slog.Warn("tunnel: could not determine tunnel state; saving anyway",
			"category", "tunnel", "tunnel", name, "error", stateErr)
	} else if active {
		slog.Warn("tunnel: config edited while connected — reconnect to apply",
			"category", "tunnel", "tunnel", name)
	}
	cfg, err := config.Parse(content)
	if err != nil {
		slog.Warn("tunnel: config rejected (parse)", "category", "tunnel", "tunnel", name, "error", err)
		return err
	}
	result := config.Validate(cfg)
	if !result.IsValid() {
		msgs := strings.Join(result.ErrorMessages(), "; ")
		slog.Warn("tunnel: config rejected (validation)", "category", "tunnel", "tunnel", name, "errors", msgs)
		return fmt.Errorf("validation failed: %s", msgs)
	}
	cfg.Name = name
	if err := s.tunnelStore.Save(cfg); err != nil {
		slog.Warn("tunnel: save failed", "category", "tunnel", "tunnel", name, "error", err)
		return err
	}
	slog.Info("tunnel: config saved", "category", "tunnel", "tunnel", name, "bytes", len(content))
	// Conflict Policy — Save: always allowed, warnings logged (principle
	// 22). A conflicting config is legal; the analyzer never rewrites
	// AllowedIPs and never refuses the save (principle 26).
	s.logPolicyWarnings(name)
	return nil
}

// ExportConfig returns the serialized text for display in the export dialog.
func (s *TunnelService) ExportConfig(name string) (string, error) {
	return s.GetConfigText(name)
}

// ExportTunnel shows a native save dialog and writes the .conf file.
// Returns the saved path, or empty string if the user cancelled.
func (s *TunnelService) ExportTunnel(name string) (string, error) {
	content, err := s.GetConfigText(name)
	if err != nil {
		slog.Warn("tunnel: export failed (load)", "category", "tunnel", "tunnel", name, "error", err)
		return "", err
	}
	if s.app == nil {
		slog.Warn("tunnel: export failed (app not initialized)", "category", "tunnel", "tunnel", name)
		return "", fmt.Errorf("app not initialized")
	}

	path, err := s.app.Dialog.SaveFile().
		SetFilename(name+".conf").
		AddFilter("WireGuard Config", "*.conf").
		PromptForSingleSelection()
	if err != nil {
		slog.Warn("tunnel: export failed (dialog)", "category", "tunnel", "tunnel", name, "error", err)
		return "", err
	}
	if path == "" {
		return "", nil // user cancelled
	}

	// Exported files contain private keys — write with 0600.
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		slog.Warn("tunnel: export failed (write)", "category", "tunnel", "tunnel", name, "path", path, "error", err)
		return "", err
	}
	slog.Info("tunnel: exported", "category", "tunnel", "tunnel", name, "path", path, "bytes", len(content))
	return path, nil
}

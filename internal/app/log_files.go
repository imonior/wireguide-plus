package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/imonior/wireguide-plus/internal/storage"
)

// logFileNameRe restricts ReadLogFile to plain daily-log filenames living
// directly in the log directory: no directory separators, no "..", no
// extension games. A compromised frontend can therefore only name files the
// app itself created there.
// logFileNameRe 将 ReadLogFile 限制为日志目录下的纯日志文件名：不允许目录
// 分隔符、不允许 ".."、不允许扩展名花样。被入侵的前端也只能点名应用自己
// 写在该目录下的文件。
var logFileNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.log$`)

// logFileTailBytes caps how much of a log file is returned. Daily logs grow
// unbounded during a session; shipping an entire file to the webview would
// stall the UI, so large files are tailed — the newest content is what a
// user inspecting a problem almost always wants.
// logFileTailBytes 限制返回的日志文件大小。单日日志在会话期间会持续增长，
// 把整个文件发给 WebView 会卡住界面，因此大文件只取末尾——排查问题时用户
// 几乎总是要看最新的内容。
const logFileTailBytes = 2 << 20 // 2 MiB / 2 MiB

// LogFileMeta describes one daily log file in the log directory.
// LogFileMeta 描述日志目录中的一个日志文件。
type LogFileMeta struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModUnix int64  `json:"mod_unix"`
}

// LogFileContent is the tailed content of one log file, split into lines.
// Lines keeps the raw on-disk text (time \t LEVEL \t message ...) rather
// than re-parsed fields: both the GUI and the helper write here, and their
// line shapes differ, so parsing in the viewer would silently drop fields.
// LogFileContent 是某个日志文件按行切开的尾部内容。Lines 保留磁盘上的原始
// 文本（时间 \t 级别 \t 消息 ...）而不做二次解析：GUI 与 helper 都会写这里，
// 两者的行格式不同，解析反而会静默丢字段。
type LogFileContent struct {
	Name      string   `json:"name"`
	Lines     []string `json:"lines"`
	Truncated bool     `json:"truncated"`
}

// ListLogFiles returns the daily log files currently in the log directory,
// newest first. Both the GUI ("wireguideplus-...") and the helper
// ("helper-...") write there, so both appear in the dropdown.
//
// ListLogFiles 返回日志目录中当前存在的按天滚动日志文件，最新在前。GUI
// （"wireguideplus-..."）与 helper（"helper-..."）都写在这里，下拉里两者
// 都会出现。
func (s *TunnelService) ListLogFiles() ([]LogFileMeta, error) {
	paths, err := storage.GetPaths()
	if err != nil {
		return nil, fmt.Errorf("resolve app paths: %w", err)
	}
	entries, err := os.ReadDir(paths.LogsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []LogFileMeta{}, nil
		}
		return nil, fmt.Errorf("read log dir: %w", err)
	}
	var result []LogFileMeta
	for _, e := range entries {
		if e.IsDir() || !logFileNameRe.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue // raced with retention cleanup; skip / 与保留清理竞争，跳过
		}
		result = append(result, LogFileMeta{
			Name:    e.Name(),
			Size:    info.Size(),
			ModUnix: info.ModTime().Unix(),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name > result[j].Name })
	return result, nil
}

// ReadLogFile returns the tail of one log file from the log directory, split
// into lines. Only plain filenames inside the log directory are accepted
// (see logFileNameRe). Files larger than logFileTailBytes are tailed and
// flagged Truncated.
//
// ReadLogFile 返回日志目录中某个日志文件的尾部内容并按行切开。只接受日志
// 目录下的纯文件名（见 logFileNameRe）。超过 logFileTailBytes 的文件只取
// 末尾，并标记 Truncated。
func (s *TunnelService) ReadLogFile(name string) (LogFileContent, error) {
	if !logFileNameRe.MatchString(name) {
		return LogFileContent{}, fmt.Errorf("invalid log file name: %q", name)
	}
	paths, err := storage.GetPaths()
	if err != nil {
		return LogFileContent{}, fmt.Errorf("resolve app paths: %w", err)
	}
	path := filepath.Join(paths.LogsDir, name)
	if filepath.Dir(path) != paths.LogsDir {
		return LogFileContent{}, fmt.Errorf("invalid log file name: %q", name)
	}
	info, err := os.Stat(path)
	if err != nil {
		return LogFileContent{}, fmt.Errorf("stat log file: %w", err)
	}
	if info.IsDir() {
		return LogFileContent{}, fmt.Errorf("not a log file: %q", name)
	}
	offset := int64(0)
	truncated := false
	if info.Size() > logFileTailBytes {
		offset = info.Size() - logFileTailBytes
		truncated = true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return LogFileContent{}, fmt.Errorf("read log file: %w", err)
	}
	chunk := data
	if truncated {
		chunk = data[offset:]
	}
	// Drop the first fragment: when tailing, it is a partial line cut by the
	// byte offset, not a real record.
	// 丢弃首行片段：截尾时它是被字节偏移切开的半行，不是真实记录。
	lines := strings.Split(strings.TrimRight(string(chunk), "\n"), "\n")
	if truncated && len(lines) > 0 {
		lines = lines[1:]
	}
	if lines == nil {
		lines = []string{}
	}
	return LogFileContent{Name: name, Lines: lines, Truncated: truncated}, nil
}

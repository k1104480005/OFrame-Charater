package version

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/motion"
)

// Operation log actions (versioning spec 9.3: 生成、编辑、接受、镜像替换等所有变更
// 以追加式操作日志记录).
const (
	ActionGeneration        = "generation"         // 生成 (filmstrip per basic directions)
	ActionRegeneration      = "regeneration"       // 重新生成 (5.6)
	ActionMirrorReplacement = "mirror_replacement" // 镜像方向手动替换 (3.5)
	ActionAcceptance        = "acceptance"         // 候选接受 (9.2)
	ActionVersionCommit     = "version_commit"     // 外观修订 (9.1)
	ActionRollback          = "rollback"           // 回退到历史点 (9.4)
	ActionEdit              = "edit"               // 轻量编辑 (7.x, reserved)
)

// oplogMu serializes appends to the append-only operation log (single-user
// desktop app; the log file itself is append-only).
var oplogMu sync.Mutex

// LogEntry is one append-only operation log entry (versioning spec 9.3). The
// snapshot carries the package's mutable content state AT this point so a
// rollback can restore any historical point (9.4: 身份包内容恢复该点状态、后续日志
// 保留).
type LogEntry struct {
	Seq      int             `json:"seq"`
	At       string          `json:"at"`
	Action   string          `json:"action"`
	Payload  json.RawMessage `json:"payload,omitempty"`
	Snapshot StateSnapshot   `json:"snapshot"`
}

// StateSnapshot is the identity package content state at one log point
// (motions.json, the current version's assets index, and the candidate
// history index; the current version id lives in the manifest).
type StateSnapshot struct {
	CurrentVersion   string `json:"currentVersion,omitempty"`
	Motions          string `json:"motions,omitempty"`          // motions.json content ("" = none)
	AssetsIndex      string `json:"assetsIndex,omitempty"`      // versions/<current>/assets/index.json
	CandidateHistory string `json:"candidateHistory,omitempty"` // candidates/index.json
}

// logPath resolves the operation log file path of a package.
func logPath(p *identity.Package) string {
	ref := p.Manifest().References.OperationLog
	if ref == "" {
		ref = identity.DefaultReferences().OperationLog
	}
	return filepath.Join(p.Root(), filepath.FromSlash(ref))
}

// Append records one operation log entry with the package's post-action state
// snapshot and returns the entry (its Seq is the historical point). The log is
// append-only: entries are never rewritten or deleted (追加式操作日志).
func Append(p *identity.Package, action string, payload any) (LogEntry, error) {
	oplogMu.Lock()
	defer oplogMu.Unlock()

	entries, err := Entries(p)
	if err != nil {
		return LogEntry{}, err
	}
	snap, err := captureSnapshot(p)
	if err != nil {
		return LogEntry{}, err
	}
	var raw json.RawMessage
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return LogEntry{}, fmt.Errorf("version: encode log payload: %w", err)
		}
		raw = data
	}
	entry := LogEntry{
		Seq:      len(entries) + 1,
		At:       time.Now().UTC().Format(time.RFC3339),
		Action:   action,
		Payload:  raw,
		Snapshot: snap,
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return LogEntry{}, fmt.Errorf("version: encode log entry: %w", err)
	}
	f, err := os.OpenFile(logPath(p), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return LogEntry{}, fmt.Errorf("version: open operation log: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return LogEntry{}, fmt.Errorf("version: append operation log: %w", err)
	}
	return entry, nil
}

// Entries reads all operation log entries in order (append-only read).
func Entries(p *identity.Package) ([]LogEntry, error) {
	f, err := os.Open(logPath(p))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("version: open operation log: %w", err)
	}
	defer f.Close()
	var out []LogEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e LogEntry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("version: parse operation log line: %w", err)
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("version: read operation log: %w", err)
	}
	return out, nil
}

// captureSnapshot reads the package's mutable content files into a snapshot.
func captureSnapshot(p *identity.Package) (StateSnapshot, error) {
	snap := StateSnapshot{}
	m := p.Manifest()
	snap.CurrentVersion = m.Versions.Current

	if data, err := os.ReadFile(motion.NewStore(p.Root()).Path()); err == nil {
		snap.Motions = string(data)
	}
	if snap.CurrentVersion != "" {
		for _, v := range m.Versions.Items {
			if v.ID == snap.CurrentVersion && v.AssetsRef != "" {
				assetsIndex := filepath.Join(p.Root(), filepath.FromSlash(v.AssetsRef), "index.json")
				if data, err := os.ReadFile(assetsIndex); err == nil {
					snap.AssetsIndex = string(data)
				}
				break
			}
		}
	}
	if h, err := p.LoadCandidateHistory(); err == nil {
		if data, err := json.Marshal(h); err == nil {
			snap.CandidateHistory = string(data)
		}
	}
	return snap, nil
}

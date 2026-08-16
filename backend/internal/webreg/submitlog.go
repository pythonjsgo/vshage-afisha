package webreg

import (
	"encoding/json"
	"log"
	"os"
	"sync"
)

// SubmitLog is the belt-and-braces copy of every submission.
//
// Why it exists: the database is the system of record, but a registration
// that fails to insert during an announcement burst is a person we lost and
// cannot ask again. Every submission is therefore written out BEFORE the
// insert is attempted, as one self-contained JSON line that can be replayed.
//
// stdout is always used because the container's json-file driver persists it
// on the host (afisha-backend keeps 3×10MB — thousands of lines). Loki is NOT
// relied on here: PROD log shipping is known broken (umbrella rules/pitfalls.md
// §Инфра), so anything that depends on Loki being up would be theatre.
// A file path additionally mirrors the lines onto a mounted volume.
type SubmitLog struct {
	mu   sync.Mutex
	file *os.File
	path string
}

// NewSubmitLog opens the optional mirror file. A path that cannot be opened is
// reported loudly and degrades to stdout-only rather than failing startup —
// losing the mirror is bad, refusing to serve the page is worse.
func NewSubmitLog(path string) *SubmitLog {
	sl := &SubmitLog{path: path}
	if path == "" {
		return sl
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		log.Printf("webreg: submit log file %q unavailable, using stdout only: %v", path, err)
		return sl
	}
	sl.file = f
	log.Printf("webreg: mirroring submissions to %s", path)
	return sl
}

// Record writes one line. Never returns an error: a logging failure must not
// abort a registration.
func (s *SubmitLog) Record(entry map[string]any) {
	line, err := json.Marshal(entry)
	if err != nil {
		log.Printf("webreg: WEBREG_SUBMIT marshal failed: %v (entry=%v)", err, entry)
		return
	}
	log.Printf("WEBREG_SUBMIT %s", line)

	if s.file == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.file.Write(append(line, '\n')); err != nil {
		log.Printf("webreg: submit log write failed: %v", err)
	}
}

func (s *SubmitLog) Close() error {
	if s.file == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.file.Close()
}

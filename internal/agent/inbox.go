package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
)

// inboxDirName is where the prompts nat sends to its agents wait for the
// embedded mod to submit them, one directory per session, under nat's state
// directory beside agent-status/ and mods/.
const inboxDirName = "agent-inbox"

// inboxEnv is the variable every session nat launches carries naming its own
// inbox directory, set by new-session's -e beside PATH. The mod reads its
// inbox from it — the state directory is nat's to resolve, never the mod's —
// and [Tmux.SendPrompt] reads it back off the session with show-environment,
// which is how a send tells a session that has an inbox from one launched
// before this nat, or on a tmux too old for -e, which has none.
const inboxEnv = "NAT_INBOX"

// inboxWait is how long a send waits for the mod to take its file before
// pasting instead, and inboxPoll how often it looks. The mod polls once a
// second, so three seconds is a mod that is not there. inboxNow names each
// file. Variables so a test need not wait them out.
var (
	inboxWait = 3 * time.Second
	inboxPoll = 200 * time.Millisecond
	inboxNow  = time.Now
)

// InboxDir is the directory the prompts for session wait in.
func InboxDir(session string) (string, error) {
	dir, err := logging.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, inboxDirName, session), nil
}

// inboxEnvArgs is the -e flag naming session's inbox, or nothing where its
// directory cannot be resolved: that session then has no inbox, and every
// send to it pastes, as before there was one.
func inboxEnvArgs(session string) []string {
	dir, err := InboxDir(session)
	if err != nil {
		logging.Action("agent inbox disabled for a launch", "session", session, "error", err.Error())
		return nil
	}
	return []string{"-e", inboxEnv + "=" + dir}
}

// sessionInbox reads the inbox directory session was launched with off its
// tmux environment: "" for a session that has none, or one that could not be
// read — a dead session among them, which the paste then fails on with the
// better error.
func (t *Tmux) sessionInbox(session string) string {
	out, err := t.run("show-environment", "-t", session, inboxEnv)
	if err != nil {
		return ""
	}
	dir, ok := strings.CutPrefix(strings.TrimSpace(out), inboxEnv+"=")
	if !ok || !filepath.IsAbs(dir) {
		return ""
	}
	return dir
}

// deliverToInbox hands text to the mod through dir and reports whether it took
// it: the file is written under a temp name and renamed into place, so the mod
// never reads half a prompt, then waited on until the mod has removed it. A
// file still there once [inboxWait] is up — no mod in that session, an older
// Claude Code, a session gone — is removed again, and false says to paste:
// the text then reaches the agent once, by the paste, never twice.
//
// The name is the send's time in Unix nanoseconds, so the mod's name order is
// the order they were sent in.
func deliverToInbox(dir, text string) (bool, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, fmt.Errorf("make the agent inbox: %w", err)
	}
	name := strconv.FormatInt(inboxNow().UnixNano(), 10)
	path := filepath.Join(dir, name+".md")
	tmp := filepath.Join(dir, "."+name+".tmp")
	if err := os.WriteFile(tmp, []byte(text), 0o600); err != nil {
		_ = os.Remove(tmp)
		return false, fmt.Errorf("write to the agent inbox: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return false, fmt.Errorf("write to the agent inbox: %w", err)
	}
	for deadline := time.Now().Add(inboxWait); ; {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(inboxPoll)
	}
	// A file gone by the time it is removed was taken between the last look
	// and the removal.
	return errors.Is(os.Remove(path), os.ErrNotExist), nil
}

package schoolconn

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// LoginManager owns one interactive login helper process. The helper (a small
// Node/Playwright script) opens the dedicated Edge window, waits for the user
// to log in and hands the school-domain cookies over the private stdout pipe.
// It never records traffic, keystrokes or screenshots.
type LoginManager struct {
	// ScriptPath is the absolute path to helper.mjs.
	ScriptPath string
	// NodePath is the node executable; empty resolves "node" from PATH.
	NodePath string
	// EdgePath is the discovered Edge executable passed to the helper.
	EdgePath string
	// LoginURL is the page the helper opens.
	LoginURL string
	// Origin is the only origin cookies are collected from.
	Origin string
	// ProfileRoot is the managed root under which each task gets
	// <login_id>/; only directories below it are ever cleaned.
	ProfileRoot string
	// StderrDir receives per-task helper stderr logs for troubleshooting.
	StderrDir string
	Log       *slog.Logger
	Now       func() time.Time
}

// LoginOutcome is what a finished helper run produced.
type LoginOutcome struct {
	Cookies []Cookie
	URL     string
	// Failed reports that the helper ended without cookies (window closed,
	// launch failure, deadline). Message is safe for display.
	Failed  bool
	Message string
}

type helperEvent struct {
	Event   string   `json:"event"`
	URL     string   `json:"url,omitempty"`
	Message string   `json:"message,omitempty"`
	Cookies []Cookie `json:"cookies,omitempty"`
}

// Available reports whether the helper script and Edge executable exist.
func (m *LoginManager) Available() bool {
	if m == nil || m.ScriptPath == "" || m.EdgePath == "" {
		return false
	}
	if m.NodePath != "" {
		if _, err := os.Stat(m.NodePath); err != nil {
			return false
		}
	} else if _, err := exec.LookPath("node"); err != nil {
		return false
	}
	_, err := os.Stat(m.ScriptPath)
	return err == nil
}

// Run launches the helper and waits for the cookie line, failure, the context
// deadline or cancellation. The profile directory is owned by the caller via
// ProfileFor/CleanupProfile; Run only manages the process.
func (m *LoginManager) Run(ctx context.Context, loginID string, deadline time.Time) (LoginOutcome, error) {
	profileDir := m.ProfileFor(loginID)
	if profileDir == "" {
		return LoginOutcome{Failed: true, Message: "login id invalid"}, fmt.Errorf("invalid login id")
	}
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return LoginOutcome{Failed: true, Message: "无法创建登录目录"}, err
	}

	node := m.NodePath
	if node == "" {
		resolved, err := exec.LookPath("node")
		if err != nil {
			return LoginOutcome{Failed: true, Message: "运行环境缺少 node"}, err
		}
		node = resolved
	}

	cmd := exec.Command(node, m.ScriptPath,
		"--profile", profileDir,
		"--url", m.LoginURL,
		"--origin", m.Origin,
		"--deadline", deadline.UTC().Format(time.RFC3339),
		"--edge", m.EdgePath,
	)
	cmd.Dir = filepath.Dir(m.ScriptPath)
	// The helper hands cookies over stdout; nothing of it may reach logs, so
	// stdout stays a pipe owned here and stderr goes to a per-task file.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return LoginOutcome{Failed: true, Message: "无法启动登录助手"}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return LoginOutcome{Failed: true, Message: "无法启动登录助手"}, err
	}
	if err := os.MkdirAll(m.StderrDir, 0o700); err != nil {
		return LoginOutcome{Failed: true, Message: "无法创建日志目录"}, err
	}
	stderrFile, err := os.Create(filepath.Join(m.StderrDir, "henu-login-"+loginID+".stderr.log"))
	if err != nil {
		return LoginOutcome{Failed: true, Message: "无法打开日志文件"}, err
	}
	defer stderrFile.Close()
	cmd.Stderr = stderrFile
	if err := cmd.Start(); err != nil {
		return LoginOutcome{Failed: true, Message: "无法启动登录助手"}, err
	}

	runCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	resultCh := make(chan LoginOutcome, 1)
	errCh := make(chan error, 1)
	go func() {
		outcome, rerr := readHelperEvents(runCtx, stdout, stdin)
		resultCh <- outcome
		errCh <- rerr
	}()

	waitErr := runCtx.Err()
	select {
	case outcome := <-resultCh:
		_ = <-errCh
		// Give the helper a moment to exit after the ack; a hung helper is
		// killed together with its browser children.
		killed := false
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			killProcessTree(cmd.Process.Pid)
			<-done
			killed = true
		}
		if killed && m.Log != nil {
			m.Log.Warn("henu login helper did not exit; killed", "login_id", loginID)
		}
		return outcome, nil
	case <-runCtx.Done():
		waitErr = runCtx.Err()
		// Ask the helper to wind down, then force-kill the whole tree.
		_, _ = io.WriteString(stdin, "cancel\n")
		killProcessTree(cmd.Process.Pid)
		_ = cmd.Wait()
		if waitErr == context.DeadlineExceeded {
			return LoginOutcome{Failed: true, Message: "登录超时"}, waitErr
		}
		return LoginOutcome{Failed: true, Message: "登录已取消"}, waitErr
	}
}

// readHelperEvents parses helper stdout lines until cookies arrive, the
// helper fails, or the context ends. Values stay here; nothing is logged.
func readHelperEvents(ctx context.Context, stdout io.Reader, stdin io.WriteCloser) (LoginOutcome, error) {
	lines := make(chan string, 8)
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	for {
		select {
		case <-ctx.Done():
			return LoginOutcome{Failed: true, Message: "登录已取消"}, ctx.Err()
		case line, ok := <-lines:
			if !ok {
				return LoginOutcome{Failed: true, Message: "登录助手意外退出"}, nil
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var ev helperEvent
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				// stdout is a strict private protocol. A malformed cookie frame
				// must fail promptly rather than wait ten minutes for a second
				// event while the helper waits for an ack. Never expose raw data.
				_, _ = io.WriteString(stdin, "cancel\n")
				return LoginOutcome{Failed: true, Message: "登录助手返回的会话格式无效"}, nil
			}
			switch ev.Event {
			case "started":
				// helper is up; keep waiting
			case "failed":
				msg := ev.Message
				if msg == "" {
					msg = "登录未完成"
				}
				return LoginOutcome{Failed: true, Message: msg}, nil
			case "cookies":
				if len(ev.Cookies) == 0 {
					return LoginOutcome{Failed: true, Message: "登录助手未取得会话"}, nil
				}
				_, _ = io.WriteString(stdin, "ack\n")
				return LoginOutcome{Cookies: ev.Cookies, URL: ev.URL}, nil
			default:
				// unknown events are ignored by protocol design
			}
		}
	}
}

// ProfileFor maps a login id to its dedicated profile directory, refusing ids
// that could escape the root.
func (m *LoginManager) ProfileFor(loginID string) string {
	if loginID == "" || strings.ContainsAny(loginID, "/\\") || strings.Contains(loginID, "..") {
		return ""
	}
	if m.ProfileRoot == "" {
		return ""
	}
	return filepath.Join(m.ProfileRoot, loginID)
}

// CleanupProfile removes the task's dedicated profile directory. It retries
// briefly because Windows keeps files locked until the browser fully exits.
func (m *LoginManager) CleanupProfile(loginID string) {
	dir := m.ProfileFor(loginID)
	if dir == "" {
		return
	}
	// Defense in depth: only directories inside the managed root are touched.
	absRoot, err := filepath.Abs(m.ProfileRoot)
	if err != nil {
		return
	}
	absDir, err := filepath.Abs(dir)
	if err != nil || !strings.HasPrefix(strings.ToLower(absDir), strings.ToLower(absRoot)+string(os.PathSeparator)) {
		return
	}
	for attempt := 0; attempt < 6; attempt++ {
		if err := os.RemoveAll(absDir); err == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	if m.Log != nil {
		m.Log.Warn("henu login profile cleanup incomplete", "login_id", loginID)
	}
}

// killProcessTree terminates the helper and its browser children. Windows
// needs taskkill /T; SIGKILL on the node pid alone would orphan Edge.
func killProcessTree(pid int) {
	if pid <= 0 {
		return
	}
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprint(pid)).Run()
		return
	}
	_ = exec.Command("kill", "-9", fmt.Sprint(pid)).Run()
}

// FindEdge locates the user's existing Edge executable via the environment
// override or the standard Windows install paths. No browser is downloaded.
func FindEdge() string {
	if p := os.Getenv("XIQUE_HENU_EDGE"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	candidates := []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

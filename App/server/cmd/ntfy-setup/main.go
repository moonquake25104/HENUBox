// Prepare a private portable ntfy configuration without modifying Windows.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"kairos/server/internal/domain/ntfy"
)

func secret(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func quote(s string) string { b, _ := json.Marshal(s); return string(b) }
func main() {
	root := flag.String("root", "", "D: workspace")
	ipText := flag.String("ip", "", "LAN IPv4")
	flag.Parse()
	if !filepath.IsAbs(*root) || !strings.EqualFold(filepath.VolumeName(*root), "D:") {
		panic("D: root required")
	}
	ip := net.ParseIP(*ipText)
	if ip == nil || ip.To4() == nil || !ip.IsPrivate() {
		panic("private IPv4 required")
	}
	dir := filepath.Join(*root, "runtime", "config", "ntfy")
	dataDir := filepath.Join(*root, "runtime", "data", "ntfy")
	for _, path := range []string{dir, dataDir, filepath.Join(*root, "runtime", "workspaces", "ntfy")} {
		if err := os.MkdirAll(path, 0700); err != nil {
			panic(err)
		}
	}
	name := filepath.Join(dir, "connection.json")
	var c ntfy.Config
	raw, err := os.ReadFile(name)
	if os.IsNotExist(err) {
		var local struct {
			AppUser string `json:"appUser"`
		}
		raw, err = os.ReadFile(filepath.Join(*root, "runtime", "config", "local-validation.json"))
		if err != nil {
			panic(err)
		}
		if err = json.Unmarshal(raw, &local); err != nil {
			panic(err)
		}
		topicBytes := make([]byte, 8)
		rand.Read(topicBytes)
		c = ntfy.Config{Owner: local.AppUser, Topic: "xique-" + hex.EncodeToString(topicBytes), PublisherURL: "http://127.0.0.1:18081", SenderUser: "xique-sender", SenderPassword: secret(32), ReaderUser: "xique-phone", ReaderPassword: secret(12)}
	} else if err != nil {
		panic(err)
	} else if err = json.Unmarshal(raw, &c); err != nil {
		panic(err)
	}
	c.ServerURL = "https://" + ip.String() + ":9443"
	c.WebURL = "https://" + ip.String() + ":8443"
	if err = c.Validate(); err != nil {
		panic(err)
	}
	bytes, _ := json.MarshalIndent(c, "", "  ")
	if err = os.WriteFile(name, bytes, 0600); err != nil {
		panic(err)
	}
	senderHash, err := bcrypt.GenerateFromPassword([]byte(c.SenderPassword), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	readerHash, err := bcrypt.GenerateFromPassword([]byte(c.ReaderPassword), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	tlsDir := filepath.Join(*root, "runtime", "config", "mobile-https")
	for _, f := range []string{"server.key", "server.crt"} {
		if _, err := os.Stat(filepath.Join(tlsDir, f)); err != nil {
			panic("Start mobile HTTPS first to prepare its certificates")
		}
	}
	yaml := fmt.Sprintf(`base-url: %s
listen-http: "127.0.0.1:18081"
listen-https: %s
key-file: %s
cert-file: %s
cache-file: %s
cache-duration: "1h"
auth-file: %s
auth-default-access: "deny-all"
auth-users: [%s, %s]
auth-access: [%s, %s]
web-root: "disable"
enable-signup: false
enable-login: false
enable-reservations: false
attachment-cache-dir: ""
firebase-key-file: ""
upstream-base-url: ""
log-level: "info"
`, quote(c.ServerURL), quote(net.JoinHostPort(ip.String(), "9443")), quote(filepath.Join(tlsDir, "server.key")), quote(filepath.Join(tlsDir, "server.crt")), quote(filepath.Join(dataDir, "cache.db")), quote(filepath.Join(dataDir, "users.db")), quote(c.SenderUser+":"+string(senderHash)+":user"), quote(c.ReaderUser+":"+string(readerHash)+":user"), quote(c.SenderUser+":"+c.Topic+":wo"), quote(c.ReaderUser+":"+c.Topic+":ro"))
	if err = os.WriteFile(filepath.Join(dir, "server.yml"), []byte(yaml), 0600); err != nil {
		panic(err)
	}
	fmt.Println("Private ntfy configuration prepared on D:. Reader settings are available only in the authenticated app.")
}

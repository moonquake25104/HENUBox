// mobile-preview serves only the production UI and API proxy on a chosen LAN IP.
// It never installs certificates in Windows or changes firewall settings.
package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	root := flag.String("root", "", "D: project root")
	address := flag.String("ip", "", "private LAN IPv4")
	initOnly := flag.Bool("init", false, "generate certificates only")
	apiPort := flag.Int("api-port", 18080, "loopback API port")
	flag.Parse()
	if !filepath.IsAbs(*root) || !strings.EqualFold(filepath.VolumeName(*root), "D:") {
		log.Fatal("root must be an absolute D: path")
	}
	ip := net.ParseIP(*address)
	if ip == nil || ip.To4() == nil || !ip.IsPrivate() {
		log.Fatal("choose a private LAN IPv4 address")
	}
	dir := filepath.Join(*root, "runtime", "config", "mobile-https")
	if err := prepare(dir, ip); err != nil {
		log.Fatal(err)
	}
	if *initOnly {
		return
	}
	target, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", *apiPort))
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		http.Error(w, "API unavailable", http.StatusBadGateway)
	}
	dist := filepath.Join(*root, "App", "dist")
	if _, err := os.Stat(filepath.Join(dist, "index.html")); err != nil {
		log.Fatal("build App before starting preview")
	}
	files := http.FileServer(http.Dir(dist))
	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path == "/downloads/ntfy-android.apk" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/vnd.android.package-archive")
			w.Header().Set("Content-Disposition", `attachment; filename="ntfy-android.apk"`)
			http.ServeFile(w, r, filepath.Join(*root, "runtime", "distribution", "mobile", "ntfy-android.apk"))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			proxy.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(405)
			return
		}
		files.ServeHTTP(w, r)
	})
	certificate, _ := os.ReadFile(filepath.Join(dir, "xique-mobile-ca.crt"))
	download := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path != "/xique-mobile-ca.crt" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-x509-ca-cert")
		w.Header().Set("Content-Disposition", `attachment; filename="xique-mobile-ca.crt"`)
		w.Write(certificate)
	})
	httpsAddr := net.JoinHostPort(ip.String(), "8443")
	httpAddr := net.JoinHostPort(ip.String(), "8800")
	secure := &http.Server{Addr: httpsAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	certServer := &http.Server{Addr: httpAddr, Handler: download, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second}
	// Bind both listeners before reporting ready; no wildcard addresses.
	tlsListener, err := net.Listen("tcp", httpsAddr)
	if err != nil {
		log.Fatal(err)
	}
	certListener, err := net.Listen("tcp", httpAddr)
	if err != nil {
		tlsListener.Close()
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		if err := secure.ServeTLS(tlsListener, filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key")); !errors.Is(err, http.ErrServerClosed) {
			log.Print("HTTPS stopped")
			stop()
		}
	}()
	go func() {
		if err := certServer.Serve(certListener); !errors.Is(err, http.ErrServerClosed) {
			log.Print("certificate server stopped")
			stop()
		}
	}()
	log.Printf("Mobile preview ready: https://%s/ (certificate trust required)", httpsAddr)
	<-ctx.Done()
	end, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	secure.Shutdown(end)
	certServer.Shutdown(end)
}

func serial() *big.Int {
	v, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if e != nil {
		panic(e)
	}
	return v
}
func writePEM(name, kind string, b []byte) error {
	return os.WriteFile(name, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: b}), 0600)
}
func prepare(dir string, ip net.IP) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	certPath, keyPath := filepath.Join(dir, "xique-mobile-ca.crt"), filepath.Join(dir, "ca.key")
	var ca *x509.Certificate
	var caKey *ecdsa.PrivateKey
	certBytes, err := os.ReadFile(certPath)
	if os.IsNotExist(err) {
		if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
			return errors.New("CA key exists without certificate; preserve and inspect it")
		}
		caKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return err
		}
		ca = &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: "Xique Assistant Local Test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(0, 0, 30), IsCA: true, BasicConstraintsValid: true, MaxPathLen: 0, MaxPathLenZero: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
		der, e := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
		if e != nil {
			return e
		}
		kb, e := x509.MarshalECPrivateKey(caKey)
		if e != nil {
			return e
		}
		if e = writePEM(keyPath, "EC PRIVATE KEY", kb); e != nil {
			return e
		}
		if e = writePEM(certPath, "CERTIFICATE", der); e != nil {
			return e
		}
		certBytes, _ = os.ReadFile(certPath)
	} else if err != nil {
		return err
	}
	block, _ := pem.Decode(certBytes)
	if block == nil {
		return errors.New("invalid CA file")
	}
	ca, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	keyBlock, _ := pem.Decode(keyBytes)
	if keyBlock == nil {
		return errors.New("invalid CA key")
	}
	caKey, err = x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return err
	}
	if time.Until(ca.NotAfter) < 24*time.Hour {
		return errors.New("test CA expired or nearly expired; remove it from phone before preparing a new CA directory")
	}
	if !caKey.PublicKey.Equal(ca.PublicKey) {
		return errors.New("CA key does not match")
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	leaf := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: "Xique LAN Test"}, IPAddresses: []net.IP{ip}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(0, 0, 7), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	kb, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		return err
	}
	if err = writePEM(filepath.Join(dir, "server.key"), "EC PRIVATE KEY", kb); err != nil {
		return err
	}
	if err = writePEM(filepath.Join(dir, "server.crt"), "CERTIFICATE", der); err != nil {
		return err
	}
	cert, _ := x509.ParseCertificate(der)
	hash := sha256.Sum256(block.Bytes)
	spki := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	info := map[string]string{"url": "https://" + ip.String() + ":8443/", "certificate_url": "http://" + ip.String() + ":8800/xique-mobile-ca.crt", "ca_sha256": strings.ToUpper(hex.EncodeToString(hash[:])), "leaf_spki": base64.StdEncoding.EncodeToString(spki[:]), "ca_expires": ca.NotAfter.Format(time.RFC3339)}
	data, _ := json.MarshalIndent(info, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "info.json"), data, 0600); err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// Command updaterehearsal holds the three helpers the update rehearsals
// (scripts/update-rehearsal-linux.sh and scripts/update-rehearsal-windows.ps1)
// need and no shell tool gives them: a throwaway feed key, a loopback HTTPS
// server for the feed, a click on a row of the tray menu, and a quit.
//
//	updaterehearsal keys -seed FILE -go FILE
//	    writes a new feed key: the base64 seed for signfeed, and a keys.go that
//	    trusts only that key under the id "rehearsal"
//	updaterehearsal feed -dir DIR -cert FILE -addr FILE
//	    serves DIR over HTTPS on a loopback port, with a certificate made on the
//	    spot; writes the certificate (PEM) and the address, then serves until killed
//	updaterehearsal click -pid PID -prefix TEXT [-wait SECONDS] [-list]
//	    Linux: clicks the first row of PID's tray menu whose label starts with
//	    TEXT, over D-Bus
//	updaterehearsal click -pid PID -id N [-wait SECONDS]
//	    Windows: clicks row N of PID's tray menu, counted from 1 in the order
//	    the app made its rows
//	updaterehearsal close -pid PID [-wait SECONDS]
//	    Windows: quits PID the way its tray's Quit does, by closing the tray
//	    window (Linux scripts send SIGTERM, which the tray takes the same way)
//
// Nothing here ships: the release workflows build cmd/htmlclay only.
package main

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// KeyID is the id the rehearsal's feed is signed under.
const KeyID = "rehearsal"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "keys":
		fs := flag.NewFlagSet("keys", flag.ExitOnError)
		seed := fs.String("seed", "", "file the base64 seed is written to")
		goFile := fs.String("go", "", "file the replacement keys.go is written to")
		fs.Parse(os.Args[2:])
		if *seed == "" || *goFile == "" || fs.NArg() != 0 {
			usage()
		}
		err = writeKeys(*seed, *goFile)
	case "feed":
		fs := flag.NewFlagSet("feed", flag.ExitOnError)
		dir := fs.String("dir", "", "folder to serve")
		cert := fs.String("cert", "", "file the certificate is written to, PEM")
		addr := fs.String("addr", "", "file the host:port is written to")
		fs.Parse(os.Args[2:])
		if *dir == "" || *cert == "" || *addr == "" || fs.NArg() != 0 {
			usage()
		}
		err = serveFeed(*dir, *cert, *addr)
	case "click":
		fs := flag.NewFlagSet("click", flag.ExitOnError)
		pid := fs.Int("pid", 0, "pid of the running HTML Clay")
		prefix := fs.String("prefix", "", "start of the row's label (Linux)")
		id := fs.Int("id", 0, "the row's number, counted from 1 (Windows)")
		wait := fs.Int("wait", 0, "seconds to wait for the row to appear")
		list := fs.Bool("list", false, "print every top-level label instead of clicking")
		fs.Parse(os.Args[2:])
		if *pid <= 0 || (*prefix == "" && *id <= 0 && !*list) || fs.NArg() != 0 {
			usage()
		}
		err = clickTray(*pid, *prefix, *id, time.Duration(*wait)*time.Second, *list)
	case "close":
		fs := flag.NewFlagSet("close", flag.ExitOnError)
		pid := fs.Int("pid", 0, "pid of the running HTML Clay")
		wait := fs.Int("wait", 0, "seconds to wait for the tray window to appear")
		fs.Parse(os.Args[2:])
		if *pid <= 0 || fs.NArg() != 0 {
			usage()
		}
		err = closeTray(*pid, time.Duration(*wait)*time.Second)
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "updaterehearsal:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: updaterehearsal keys -seed FILE -go FILE | feed -dir DIR -cert FILE -addr FILE | click -pid PID (-prefix TEXT | -id N) [-wait SECONDS] [-list] | close -pid PID [-wait SECONDS]")
	os.Exit(2)
}

// writeKeys makes the key pair one rehearsal run signs and trusts.
func writeKeys(seedFile, goFile string) error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := os.WriteFile(seedFile, []byte(base64.StdEncoding.EncodeToString(priv.Seed())+"\n"), 0o600); err != nil {
		return err
	}
	return os.WriteFile(goFile, []byte(keysSource(pub)), 0o644)
}

// keysSource is internal/update/keys.go with the one rehearsal key in it.
func keysSource(pub ed25519.PublicKey) string {
	bytes := make([]string, len(pub))
	for i, b := range pub {
		bytes[i] = fmt.Sprintf("0x%02x", b)
	}
	return "package update\n\nimport \"crypto/ed25519\"\n\n" +
		"// TrustedKeys in a rehearsal build holds one throwaway key and nothing else.\n" +
		"var TrustedKeys = map[string]ed25519.PublicKey{\n" +
		"\t\"" + KeyID + "\": {" + strings.Join(bytes, ", ") + "},\n" +
		"}\n"
}

// serveFeed serves dir over HTTPS on 127.0.0.1. The feed refuses an artifact
// whose address is not https, so the rehearsal needs a certificate the old app
// trusts: this one, named to it by SSL_CERT_FILE.
func serveFeed(dir, certFile, addrFile string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		return err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "HTML Clay update rehearsal"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	if err := os.WriteFile(addrFile, []byte(ln.Addr().String()+"\n"), 0o644); err != nil {
		return err
	}
	logged := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("%s %s %s\n", time.Now().UTC().Format(time.RFC3339), r.Method, r.URL.Path)
		http.FileServer(http.Dir(dir)).ServeHTTP(w, r)
	})
	srv := &http.Server{Handler: logged, TLSConfig: &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
	}}
	return srv.ServeTLS(ln, "", "")
}

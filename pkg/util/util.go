package util

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"net/url"
	"strings"
	"time"
)

func GetCertPool(paths []string, isDefaultPoolNeeded bool) (*x509.CertPool, error) {

	pool := x509.NewCertPool()
	var err error

	if isDefaultPoolNeeded {
		pool, err = x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("root system ca bundle could not be read - %s", err)
		}
	}

	for _, path := range paths {
		// Cert paths are a configurable option
		data, err := ioutil.ReadFile(path) // #nosec G304
		if err != nil {
			return nil, fmt.Errorf("certificate authority file (%s) could not be read - %s", path, err)
		}
		if !pool.AppendCertsFromPEM(data) {
			return nil, fmt.Errorf("loading certificate authority (%s) failed", path)
		}
	}
	return pool, nil
}

// https://golang.org/src/crypto/tls/generate_cert.go as a function
func GenerateCert(ipaddr string) ([]byte, []byte, error) {
	var err error

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	keyBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, keyBytes, err
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, keyBytes, err
	}

	notBefore := time.Now()
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"OAuth2 Proxy Test Suite"},
		},
		NotBefore: notBefore,
		NotAfter:  notBefore.Add(time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,

		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},

		IPAddresses: []net.IP{net.ParseIP(ipaddr)},
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	return certBytes, keyBytes, err
}

// SplitHostPort separates host and port. If the port is not valid, it returns
// the entire input as host, and it doesn't check the validity of the host.
// Unlike net.SplitHostPort, but per RFC 3986, it requires ports to be numeric.
// *** taken from net/url, modified validOptionalPort() to accept ":*"
func SplitHostPort(hostport string) (host, port string) {
	host = hostport

	colon := strings.LastIndexByte(host, ':')
	if colon != -1 && validOptionalPort(host[colon:]) {
		host, port = host[:colon], host[colon+1:]
	}

	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}

	return
}

// validOptionalPort reports whether port is either an empty string
// or matches /^:\d*$/
// *** taken from net/url, modified to accept ":*"
func validOptionalPort(port string) bool {
	if port == "" || port == ":*" {
		return true
	}
	if port[0] != ':' {
		return false
	}
	for _, b := range port[1:] {
		if b < '0' || b > '9' {
			return false
		}
	}
	return true
}

// IsEndpointAllowed checks whether the endpoint URL is allowed based
// on an allowed domains list.
func IsEndpointAllowed(endpoint *url.URL, allowedDomains []string) bool {
	hostname := endpoint.Hostname()

	for _, allowedDomain := range allowedDomains {
		allowedHost, allowedPort := SplitHostPort(allowedDomain)
		if allowedHost == "" {
			continue
		}

		if isHostnameAllowed(hostname, allowedHost) {
			// the domain names match, now validate the ports
			// if the allowed domain's port is '*', allow all ports
			// if the allowed domain contains a specific port, only allow that port
			// if the allowed domain doesn't contain a port at all, only allow empty redirect ports ie http and https
			redirectPort := endpoint.Port()
			if allowedPort == "*" ||
				allowedPort == redirectPort ||
				(allowedPort == "" && redirectPort == "") {
				return true
			}
		}
	}

	return false
}

func isHostnameAllowed(hostname, allowedHost string) bool {
	// check if we have a perfect match between hostname and allowedHost
	if hostname == strings.TrimPrefix(allowedHost, ".") ||
		hostname == strings.TrimPrefix(allowedHost, "*.") {
		return true
	}

	// check if hostname is a sub domain of the allowedHost
	if (strings.HasPrefix(allowedHost, ".") && strings.HasSuffix(hostname, allowedHost)) ||
		(strings.HasPrefix(allowedHost, "*.") && strings.HasSuffix(hostname, allowedHost[1:])) {
		return true
	}

	return false
}

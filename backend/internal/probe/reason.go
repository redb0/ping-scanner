package probe

import (
	"crypto/x509"
	"errors"
	"net"
	"net/url"
	"strings"
	"syscall"
	"unicode/utf8"
)

const (
	reasonEllipsis = "..."
	reasonMaxRunes = 80
)

func reason(probeError error) string {
	if probeError == nil {
		return ""
	}

	var netErr net.Error
	if errors.As(probeError, &netErr) && netErr.Timeout() {
		return "timeout"
	}

	var dnsErr *net.DNSError
	if errors.As(probeError, &dnsErr) {
		return "DNS: " + dnsErr.Err
	}

	if errors.Is(probeError, syscall.ECONNREFUSED) {
		return "connection refused"
	}

	var unknownAuth x509.UnknownAuthorityError
	if errors.As(probeError, &unknownAuth) {
		return "TLS: certificate signed by unknown authority"
	}

	var certInvalid x509.CertificateInvalidError
	if errors.As(probeError, &certInvalid) {
		if certInvalid.Reason == x509.Expired {
			return "TLS: certificate has expired"
		}
		return "TLS: certificate is invalid"
	}

	var hostname x509.HostnameError
	if errors.As(probeError, &hostname) {
		return "TLS: " + strings.TrimPrefix(hostname.Error(), "x509: ")
	}

	var urlErr *url.Error
	if errors.As(probeError, &urlErr) && urlErr.Op != "parse" && urlErr.Err != nil {
		return urlErr.Err.Error()
	}
	return probeError.Error()
}

func clipReason(text string) string {
	if utf8.RuneCountInString(text) <= reasonMaxRunes {
		return text
	}
	runes := []rune(text)
	keep := reasonMaxRunes - utf8.RuneCountInString(reasonEllipsis)
	return string(runes[:keep]) + reasonEllipsis
}

package utils

import (
	"crypto/md5"
	"encoding/base64"
	"io"
	"strings"

	"github.com/aporeto-inc/trireme-lib/enforcer/utils/tokens"
)

// GenerateNodeName generates a valid Trireme ID for this instance of the enforcer.
// It uses an MD5 base algorithm and is adjusted to the maximum length for trireme.
func GenerateNodeName(kubeNodeName string) string {
	h := md5.New()
	io.WriteString(h, kubeNodeName)
	md5Result := h.Sum(nil)
	b64Result := strings.ToLower(base64.StdEncoding.EncodeToString(md5Result))
	triremeNodeName := "trireme-" + b64Result

	// Checking statically if the node name is not more than the maximum ServerID
	// length supported by Trireme.
	if len(triremeNodeName) > tokens.MaxServerName {
		triremeNodeName = triremeNodeName[len(triremeNodeName)-tokens.MaxServerName:]
	}

	return triremeNodeName
}

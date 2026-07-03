package edgediscovery

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	"github.com/pkg/errors"
)

// DialEdge makes a TLS connection to a Cloudflare edge node
func DialEdge(
	ctx context.Context,
	timeout time.Duration,
	tlsConfig *tls.Config,
	edgeTCPAddr *net.TCPAddr,
	localIP net.IP,
) (net.Conn, error) {
	return DialEdgeWithProxy(ctx, timeout, tlsConfig, edgeTCPAddr, localIP)
}

// DialError is an error returned from DialEdge
type DialError struct {
	cause error
}

func newDialError(err error, message string) error {
	return DialError{cause: errors.Wrap(err, message)}
}

func (e DialError) Error() string {
	return e.cause.Error()
}

func (e DialError) Cause() error {
	return e.cause
}

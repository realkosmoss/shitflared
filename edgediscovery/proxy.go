package edgediscovery

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/net/proxy"
)

func init() {
	proxy.RegisterDialerType("http", newHTTPConnectDialer)
	proxy.RegisterDialerType("https", newHTTPConnectDialer)
}

type httpConnectDialer struct {
	proxyURL *url.URL
	forward  proxy.Dialer
}

func newHTTPConnectDialer(proxyURL *url.URL, forward proxy.Dialer) (proxy.Dialer, error) {
	return &httpConnectDialer{proxyURL: proxyURL, forward: forward}, nil
}

func (d *httpConnectDialer) Dial(network, addr string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, addr)
}

func (d *httpConnectDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, errors.Errorf("http CONNECT proxy does not support network %q", network)
	}

	proxyConn, err := d.forward.Dial(network, d.proxyURL.Host)
	if err != nil {
		return nil, errors.Wrap(err, "connecting to HTTP proxy")
	}

	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Host: addr},
		Host:   addr,
		Header: make(http.Header),
	}
	if d.proxyURL.User != nil {
		auth := base64.StdEncoding.EncodeToString([]byte(d.proxyURL.User.String()))
		req.Header.Set("Proxy-Authorization", "Basic "+auth)
	}

	if err := req.Write(proxyConn); err != nil {
		proxyConn.Close()
		return nil, errors.Wrap(err, "writing CONNECT request")
	}

	resp, err := http.ReadResponse(bufio.NewReader(proxyConn), req)
	if err != nil {
		proxyConn.Close()
		return nil, errors.Wrap(err, "reading CONNECT response")
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		proxyConn.Close()
		return nil, errors.Errorf("proxy CONNECT returned %d %s", resp.StatusCode, resp.Status)
	}

	return proxyConn, nil
}

func DialEdgeWithProxy(
	ctx context.Context,
	timeout time.Duration,
	tlsConfig *tls.Config,
	edgeTCPAddr *net.TCPAddr,
	localIP net.IP,
) (net.Conn, error) {
	forward := &net.Dialer{}
	if localIP != nil {
		forward.LocalAddr = &net.TCPAddr{IP: localIP, Port: 0}
	}
	pd := proxy.FromEnvironmentUsing(forward)

	edgeConn, err := pd.Dial("tcp", edgeTCPAddr.String())
	if err != nil {
		return nil, newDialError(err, "dial error")
	}

	tlsEdgeConn := tls.Client(edgeConn, tlsConfig)
	tlsEdgeConn.SetDeadline(time.Now().Add(timeout))
	if err := tlsEdgeConn.Handshake(); err != nil {
		edgeConn.Close()
		return nil, newDialError(err, "TLS handshake with edge error")
	}
	tlsEdgeConn.SetDeadline(time.Time{})
	return tlsEdgeConn, nil
}



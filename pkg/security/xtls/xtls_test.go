package xtls_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"

	"github.com/omeyang/xkit/pkg/security/xtls"
)

// certBundle 是一组自签 CA 及其签发的 server/client 证书。
type certBundle struct {
	caPEM     []byte
	serverPEM []byte
	serverKey []byte
	clientPEM []byte
	clientKey []byte
}

func newCertBundle(t *testing.T, serverDNS string) certBundle {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "xtls-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	serverPEM, serverKey := issue(t, caCert, caKey, pkix.Name{CommonName: "xtls-test-server"},
		[]string{serverDNS}, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth})
	clientPEM, clientKey := issue(t, caCert, caKey, pkix.Name{CommonName: "xtls-test-client"},
		nil, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth})

	return certBundle{
		caPEM:     encodePEM("CERTIFICATE", caDER),
		serverPEM: serverPEM,
		serverKey: serverKey,
		clientPEM: clientPEM,
		clientKey: clientKey,
	}
}

func issue(t *testing.T, caCert *x509.Certificate, caKey *ecdsa.PrivateKey, subject pkix.Name,
	dnsNames []string, extKeyUsage []x509.ExtKeyUsage) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      subject,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  extKeyUsage,
		DNSNames:     dnsNames,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return encodePEM("CERTIFICATE", der), encodePEM("PRIVATE KEY", keyDER)
}

func encodePEM(blockType string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
}

func TestEnabledFalseReturnsInsecure(t *testing.T) {
	c := xtls.Config{Enabled: false}

	sc, err := xtls.ServerCredentials(c)
	require.NoError(t, err)
	assert.Equal(t, insecure.NewCredentials().Info().SecurityProtocol,
		sc.Info().SecurityProtocol)

	cc, err := xtls.ClientCredentials(c)
	require.NoError(t, err)
	assert.Equal(t, insecure.NewCredentials().Info().SecurityProtocol,
		cc.Info().SecurityProtocol)
}

func TestBuildServer_MissingMaterial(t *testing.T) {
	_, err := xtls.BuildServerTLSConfig(xtls.Config{Enabled: true})
	assert.ErrorIs(t, err, xtls.ErrMissingCert)

	b := newCertBundle(t, "localhost")
	_, err = xtls.BuildServerTLSConfig(xtls.Config{
		Enabled: true,
		CertPEM: b.serverPEM,
	})
	assert.ErrorIs(t, err, xtls.ErrMissingKey)

	_, err = xtls.BuildServerTLSConfig(xtls.Config{
		Enabled:           true,
		CertPEM:           b.serverPEM,
		KeyPEM:            b.serverKey,
		RequireClientCert: true,
	})
	assert.ErrorIs(t, err, xtls.ErrMissingCA)
}

func TestBuildServer_InvalidCA(t *testing.T) {
	b := newCertBundle(t, "localhost")
	_, err := xtls.BuildServerTLSConfig(xtls.Config{
		Enabled:           true,
		CertPEM:           b.serverPEM,
		KeyPEM:            b.serverKey,
		CAPEM:             []byte("not a pem"),
		RequireClientCert: true,
	})
	assert.ErrorIs(t, err, xtls.ErrInvalidCA)
}

func TestBuildServer_FileLoading(t *testing.T) {
	b := newCertBundle(t, "localhost")
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")
	caPath := filepath.Join(dir, "ca.crt")
	require.NoError(t, os.WriteFile(certPath, b.serverPEM, 0o600))
	require.NoError(t, os.WriteFile(keyPath, b.serverKey, 0o600))
	require.NoError(t, os.WriteFile(caPath, b.caPEM, 0o600))

	cfg, err := xtls.BuildServerTLSConfig(xtls.Config{
		Enabled:           true,
		CertFile:          certPath,
		KeyFile:           keyPath,
		CAFile:            caPath,
		RequireClientCert: true,
	})
	require.NoError(t, err)
	assert.Equal(t, tls.RequireAndVerifyClientCert, cfg.ClientAuth)
	assert.NotNil(t, cfg.ClientCAs)
	assert.Len(t, cfg.Certificates, 1)
}

func TestBuildServer_InlineOverridesFile(t *testing.T) {
	b := newCertBundle(t, "localhost")
	cfg, err := xtls.BuildServerTLSConfig(xtls.Config{
		Enabled:  true,
		CertFile: "/does/not/exist",
		KeyFile:  "/does/not/exist",
		CertPEM:  b.serverPEM,
		KeyPEM:   b.serverKey,
	})
	require.NoError(t, err)
	assert.Equal(t, tls.NoClientCert, cfg.ClientAuth) // 未给 CA 且默认未强制
}

func TestBuildServer_SingleWayTLS(t *testing.T) {
	b := newCertBundle(t, "localhost")
	cfg, err := xtls.BuildServerTLSConfig(xtls.Config{
		Enabled: true,
		CertPEM: b.serverPEM,
		KeyPEM:  b.serverKey,
	})
	require.NoError(t, err)
	assert.Equal(t, tls.NoClientCert, cfg.ClientAuth)
	assert.Nil(t, cfg.ClientCAs)
}

func TestBuildClient_MinVersionFloor(t *testing.T) {
	b := newCertBundle(t, "srv")
	cfg, err := xtls.BuildClientTLSConfig(xtls.Config{
		Enabled:    true,
		CAPEM:      b.caPEM,
		MinVersion: tls.VersionTLS10, // 低于基线
	})
	require.NoError(t, err)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)

	cfg13, err := xtls.BuildClientTLSConfig(xtls.Config{
		Enabled:    true,
		CAPEM:      b.caPEM,
		MinVersion: tls.VersionTLS13,
	})
	require.NoError(t, err)
	assert.Equal(t, uint16(tls.VersionTLS13), cfg13.MinVersion)
}

// TestMTLSHandshake 通过 bufconn 验证完整 mTLS 握手链路。
func TestMTLSHandshake(t *testing.T) {
	const srvName = "xtls.test.local"
	b := newCertBundle(t, srvName)

	serverCreds, err := xtls.ServerCredentials(xtls.Config{
		Enabled:           true,
		CertPEM:           b.serverPEM,
		KeyPEM:            b.serverKey,
		CAPEM:             b.caPEM,
		RequireClientCert: true,
	})
	require.NoError(t, err)

	clientCreds, err := xtls.ClientCredentials(xtls.Config{
		Enabled:    true,
		CertPEM:    b.clientPEM,
		KeyPEM:     b.clientKey,
		CAPEM:      b.caPEM,
		ServerName: srvName,
	})
	require.NoError(t, err)

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.Creds(serverCreds))
	healthgrpc.RegisterHealthServer(srv, health.NewServer())
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(lis) }()
	t.Cleanup(func() {
		srv.Stop()
		if err := <-serveErr; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Logf("server exited: %v", err)
		}
	})

	dialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(clientCreds),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Logf("close conn: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := healthgrpc.NewHealthClient(conn).Check(ctx, &healthgrpc.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, healthgrpc.HealthCheckResponse_SERVING, resp.GetStatus())
}

// TestMTLSHandshake_ClientCertMissing 验证缺失客户端证书时握手失败。
func TestMTLSHandshake_ClientCertMissing(t *testing.T) {
	const srvName = "xtls.test.local"
	b := newCertBundle(t, srvName)

	serverCreds, err := xtls.ServerCredentials(xtls.Config{
		Enabled:           true,
		CertPEM:           b.serverPEM,
		KeyPEM:            b.serverKey,
		CAPEM:             b.caPEM,
		RequireClientCert: true,
	})
	require.NoError(t, err)

	// 客户端仅配 CA，不带 client 证书
	clientCreds, err := xtls.ClientCredentials(xtls.Config{
		Enabled:    true,
		CAPEM:      b.caPEM,
		ServerName: srvName,
	})
	require.NoError(t, err)

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.Creds(serverCreds))
	healthgrpc.RegisterHealthServer(srv, health.NewServer())
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(lis) }()
	t.Cleanup(func() {
		srv.Stop()
		if err := <-serveErr; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Logf("server exited: %v", err)
		}
	})

	dialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(clientCreds),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Logf("close conn: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = healthgrpc.NewHealthClient(conn).Check(ctx, &healthgrpc.HealthCheckRequest{})
	assert.Error(t, err) // 握手失败
	assert.False(t, errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil)
}

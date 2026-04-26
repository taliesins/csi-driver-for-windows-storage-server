package iscsi

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func freeTCPAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	return listener.Addr().String()
}

func TestNonBlockingGRPCServer_StartStopWait(t *testing.T) {
	server := NewNonBlockingGRPCServer().(*nonBlockingGRPCServer)
	driver := NewDriver("node-001", "tcp://"+freeTCPAddress(t))

	server.Start("tcp://"+freeTCPAddress(t), NewDefaultIdentityServer(driver), nil, nil)
	require.Eventually(t, func() bool {
		return server.server != nil
	}, time.Second, 10*time.Millisecond)

	server.Stop()
	done := make(chan struct{})
	go func() {
		server.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("server Wait did not return after Stop")
	}
}

func TestNonBlockingGRPCServer_ForceStopBeforeStartIsNoop(t *testing.T) {
	server := NewNonBlockingGRPCServer()

	assert.NotPanics(t, func() {
		server.ForceStop()
		server.Stop()
	})
}

func TestNonBlockingGRPCServer_ServeInvalidEndpoint(t *testing.T) {
	server := &nonBlockingGRPCServer{}

	err := server.serve("npipe://pipe/csi", nil, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid endpoint")
}

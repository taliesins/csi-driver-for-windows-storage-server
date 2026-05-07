package iscsi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// ---------------------------------------------------------------------------
// Driver tests
// ---------------------------------------------------------------------------

func TestNewDriver(t *testing.T) {
	d := NewDriver("node-001", "unix:///var/run/csi/csi.sock")

	assert.Equal(t, "windows-storage.csi.windows.microsoft.com", d.name)
	assert.Equal(t, Protocol(""), d.protocol)
	assert.Equal(t, "0.1.0", d.version)
	assert.Equal(t, "node-001", d.nodeID)
	assert.Equal(t, "unix:///var/run/csi/csi.sock", d.endpoint)
	require.NotNil(t, d.cap)
	assert.Len(t, d.cap, 6)
	require.NotNil(t, d.cscap)
	assert.Len(t, d.cscap, 7)
}

func TestNewProtocolDriver(t *testing.T) {
	tests := []struct {
		name               string
		protocol           Protocol
		wantDriver         string
		wantCaps           int
		wantControllerCaps int
		wantListSnapshots  bool
	}{
		{name: "iscsi", protocol: ProtocolISCSI, wantDriver: "windows-storage.csi.windows.microsoft.com", wantCaps: 3, wantControllerCaps: 7, wantListSnapshots: true},
		{name: "nfs", protocol: ProtocolNFS, wantDriver: "windows-storage.csi.windows.microsoft.com", wantCaps: 6, wantControllerCaps: 5, wantListSnapshots: false},
		{name: "smb", protocol: ProtocolSMB, wantDriver: "windows-storage.csi.windows.microsoft.com", wantCaps: 6, wantControllerCaps: 5, wantListSnapshots: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewProtocolDriver(tt.protocol, "node-001", "unix:///var/run/csi/csi.sock")
			assert.Equal(t, tt.wantDriver, d.name)
			assert.Equal(t, tt.protocol, d.protocol)
			assert.Len(t, d.cap, tt.wantCaps)
			assert.Len(t, d.cscap, tt.wantControllerCaps)
			assert.Equal(t, tt.wantListSnapshots, driverHasControllerCapability(d, csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS))
		})
	}
}

func TestNewProtocolDriver_UnknownDefaultsToISCSI(t *testing.T) {
	d := NewProtocolDriver(Protocol("bad-protocol"), "node-001", "unix:///var/run/csi/csi.sock")

	assert.Equal(t, driverName, d.name)
	assert.Equal(t, ProtocolISCSI, d.protocol)
	assert.Equal(t, "", d.fileShareBackend)
}

func driverHasControllerCapability(d *driver, capType csi.ControllerServiceCapability_RPC_Type) bool {
	for _, cap := range d.cscap {
		if cap.GetRpc().GetType() == capType {
			return true
		}
	}
	return false
}

func TestNewNamedDriver(t *testing.T) {
	d := NewNamedDriver("windows-storage.csi.windows.microsoft.com", "node-001", "unix:///var/run/csi/csi.sock")
	assert.Equal(t, "windows-storage.csi.windows.microsoft.com", d.name)
	assert.Equal(t, Protocol(""), d.protocol)
	assert.Equal(t, "", d.fileShareBackend)
	assert.Len(t, d.cap, 6)
	assert.True(t, driverHasControllerCapability(d, csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT))
	assert.True(t, driverHasControllerCapability(d, csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS))
}

func TestNewNamedDriver_LegacyProtocolNames(t *testing.T) {
	d := NewNamedDriver("nfs.csi.windows.microsoft.com", "node-001", "unix:///var/run/csi/csi.sock")
	assert.Equal(t, "nfs.csi.windows.microsoft.com", d.name)
	assert.Equal(t, ProtocolNFS, d.protocol)
	assert.Equal(t, fileShareBackendDirectory, d.fileShareBackend)
	assert.False(t, driverHasControllerCapability(d, csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT))
	assert.False(t, driverHasControllerCapability(d, csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS))
}

func TestNewNamedDriver_VHDXBackedFileShares(t *testing.T) {
	tests := []struct {
		name       string
		driverName string
		protocol   Protocol
	}{
		{name: "nfs", driverName: nfsVHDXDriverName, protocol: ProtocolNFS},
		{name: "smb", driverName: smbVHDXDriverName, protocol: ProtocolSMB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewNamedDriver(tt.driverName, "node-001", "unix:///var/run/csi/csi.sock")
			assert.Equal(t, tt.driverName, d.name)
			assert.Equal(t, tt.protocol, d.protocol)
			assert.Equal(t, fileShareBackendVHDX, d.fileShareBackend)
			assert.Len(t, d.cap, 6)
			assert.Len(t, d.cscap, 7)
			assert.True(t, driverHasControllerCapability(d, csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT))
			assert.True(t, driverHasControllerCapability(d, csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS))
		})
	}
}

func TestNewNamedDriver_UnknownDefaultsToISCSI(t *testing.T) {
	d := NewNamedDriver("unknown.csi.windows.microsoft.com", "node-001", "unix:///var/run/csi/csi.sock")

	assert.Equal(t, "unknown.csi.windows.microsoft.com", d.name)
	assert.Equal(t, ProtocolISCSI, d.protocol)
	assert.Equal(t, "", d.fileShareBackend)
}

func TestDriverNameAndConfigHelpers(t *testing.T) {
	tests := []struct {
		name             string
		driverName       string
		wantProtocol     Protocol
		wantShareBackend string
	}{
		{name: "consolidated", driverName: driverName, wantProtocol: ""},
		{name: "legacy iscsi", driverName: legacyISCSIDriverName, wantProtocol: ProtocolISCSI},
		{name: "nfs directory", driverName: nfsDriverName, wantProtocol: ProtocolNFS, wantShareBackend: fileShareBackendDirectory},
		{name: "smb directory", driverName: smbDriverName, wantProtocol: ProtocolSMB, wantShareBackend: fileShareBackendDirectory},
		{name: "nfs vhdx", driverName: nfsVHDXDriverName, wantProtocol: ProtocolNFS, wantShareBackend: fileShareBackendVHDX},
		{name: "smb vhdx", driverName: smbVHDXDriverName, wantProtocol: ProtocolSMB, wantShareBackend: fileShareBackendVHDX},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protocol, backend, err := driverConfigForName(tt.driverName)
			require.NoError(t, err)
			assert.Equal(t, tt.wantProtocol, protocol)
			assert.Equal(t, tt.wantShareBackend, backend)

			gotProtocol, err := protocolForDriverName(tt.driverName)
			require.NoError(t, err)
			assert.Equal(t, tt.wantProtocol, gotProtocol)
		})
	}

	_, _, err := driverConfigForName("bad-driver")
	assert.Error(t, err)

	_, err = protocolForDriverName("bad-driver")
	assert.Error(t, err)

	_, err = driverNameForProtocol(Protocol("bad-protocol"))
	assert.Error(t, err)
}

func TestNewNodeServer(t *testing.T) {
	d := NewDriver("node-001", "unix:///var/run/csi/csi.sock")
	ns := NewNodeServer(d)

	require.NotNil(t, ns)
	assert.Equal(t, d, ns.Driver)
}

func TestNewControllerServer(t *testing.T) {
	d := NewDriver("node-001", "unix:///var/run/csi/csi.sock")
	cs := NewControllerServer(d)

	require.NotNil(t, cs)
	assert.Equal(t, d, cs.Driver)
}

func TestAddVolumeCapabilityAccessModes(t *testing.T) {
	d := NewDriver("node-001", "unix:///var/run/csi/csi.sock")

	caps := d.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
	})

	require.NotNil(t, caps)
	assert.Len(t, caps, 2)
	assert.Len(t, d.cap, 2)
}

func TestAddControllerServiceCapabilities(t *testing.T) {
	d := NewDriver("node-001", "unix:///var/run/csi/csi.sock")

	d.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
	})

	assert.Len(t, d.cscap, 2)
}

func TestGetenvDefault(t *testing.T) {
	t.Setenv("CSI_TEST_ENV", "")
	assert.Equal(t, "fallback", getenvDefault("CSI_TEST_ENV", "fallback"))

	t.Setenv("CSI_TEST_ENV", " value ")
	assert.Equal(t, "value", getenvDefault("CSI_TEST_ENV", "fallback"))
}

func TestParseBoolDefault(t *testing.T) {
	tests := []struct {
		input string
		def   bool
		want  bool
	}{
		{input: "", def: true, want: true},
		{input: "true", want: true},
		{input: "YES", want: true},
		{input: "on", want: true},
		{input: "1", want: true},
		{input: "false", def: true, want: false},
		{input: "NO", def: true, want: false},
		{input: "off", def: true, want: false},
		{input: "0", def: true, want: false},
		{input: "not-bool", def: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, parseBoolDefault(tt.input, tt.def))
		})
	}
}

func TestParseDriverMode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    DriverMode
		wantErr bool
	}{
		{name: "controller", input: "controller", want: DriverModeController},
		{name: "node", input: "node", want: DriverModeNode},
		{name: "trims and lowercases", input: " Controller ", want: DriverModeController},
		{name: "empty", input: "", wantErr: true},
		{name: "unknown", input: "all", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDriverMode(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLeaderElectionConfigWithDefaults(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "kube-system")
	t.Setenv("POD_NAME", "controller-0")

	d := NewDriver("node-001", "unix:///var/run/csi/csi.sock")
	got, err := (LeaderElectionConfig{}).withDefaults(d)
	require.NoError(t, err)

	assert.Equal(t, d.name, got.LeaseName)
	assert.Equal(t, "kube-system", got.LeaseNamespace)
	assert.Equal(t, "controller-0", got.Identity)
	assert.Equal(t, defaultLeaderElectionLeaseDuration, got.LeaseDuration)
	assert.Equal(t, defaultLeaderElectionRenewDeadline, got.RenewDeadline)
	assert.Equal(t, defaultLeaderElectionRetryPeriod, got.RetryPeriod)
}

func TestLeaderElectionConfigValidation(t *testing.T) {
	d := NewDriver("node-001", "unix:///var/run/csi/csi.sock")

	t.Run("missing namespace", func(t *testing.T) {
		t.Setenv("POD_NAMESPACE", "")
		_, err := (LeaderElectionConfig{Identity: "controller-0"}).withDefaults(d)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "namespace")
	})

	t.Run("invalid duration ordering", func(t *testing.T) {
		_, err := (LeaderElectionConfig{
			LeaseNamespace: "kube-system",
			Identity:       "controller-0",
			LeaseDuration:  10 * time.Second,
			RenewDeadline:  10 * time.Second,
			RetryPeriod:    2 * time.Second,
		}).withDefaults(d)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "lease duration")
	})
}

func clearWinRMEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"WINRM_HOST",
		"WINRM_PORT",
		"WINRM_TLS",
		"WINRM_INSECURE",
		"WINRM_USER",
		"WINRM_PASSWORD",
		"WINRM_TIMEOUT",
		"WINRM_AUTH",
		"WINRM_PS_IMPORT",
	} {
		t.Setenv(key, "")
	}
}

func TestNewWinRMBackendFromEnv(t *testing.T) {
	clearWinRMEnv(t)
	t.Setenv("WINRM_HOST", "storage.example.test")
	t.Setenv("WINRM_TLS", "true")
	t.Setenv("WINRM_INSECURE", "false")
	t.Setenv("WINRM_USER", "admin")
	t.Setenv("WINRM_PASSWORD", "secret")
	t.Setenv("WINRM_TIMEOUT", "45s")
	t.Setenv("WINRM_AUTH", "ntlm")
	t.Setenv("WINRM_PS_IMPORT", "Import-Module Custom")

	backend, err := newWinRMBackendFromEnv()
	require.NoError(t, err)

	winrmBackend, ok := backend.(*WinRMBackend)
	require.True(t, ok)
	assert.Equal(t, "storage.example.test", winrmBackend.Endpoint.Host)
	assert.Equal(t, 5986, winrmBackend.Endpoint.Port)
	assert.True(t, winrmBackend.Endpoint.HTTPS)
	assert.False(t, winrmBackend.Endpoint.Insecure)
	assert.Equal(t, 45*time.Second, winrmBackend.Timeout)
	assert.Equal(t, "ntlm", winrmBackend.Auth)
	assert.Equal(t, "Import-Module Custom", winrmBackend.PSModuleImport)
}

func TestNewWinRMBackendFromEnv_CustomPortAndErrors(t *testing.T) {
	t.Run("missing host", func(t *testing.T) {
		clearWinRMEnv(t)
		_, err := newWinRMBackendFromEnv()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "WINRM_HOST")
	})

	t.Run("bad port", func(t *testing.T) {
		clearWinRMEnv(t)
		t.Setenv("WINRM_HOST", "storage.example.test")
		t.Setenv("WINRM_PORT", "bad")
		_, err := newWinRMBackendFromEnv()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid WINRM_PORT")
	})

	t.Run("missing credentials", func(t *testing.T) {
		clearWinRMEnv(t)
		t.Setenv("WINRM_HOST", "storage.example.test")
		_, err := newWinRMBackendFromEnv()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "WINRM_USER and WINRM_PASSWORD")
	})

	t.Run("custom port", func(t *testing.T) {
		clearWinRMEnv(t)
		t.Setenv("WINRM_HOST", "storage.example.test")
		t.Setenv("WINRM_PORT", "55985")
		t.Setenv("WINRM_USER", "admin")
		t.Setenv("WINRM_PASSWORD", "secret")
		backend, err := newWinRMBackendFromEnv()
		require.NoError(t, err)
		winrmBackend := backend.(*WinRMBackend)
		assert.Equal(t, 55985, winrmBackend.Endpoint.Port)
		assert.False(t, winrmBackend.Endpoint.HTTPS)
	})
}

func TestIdentityServer(t *testing.T) {
	d := NewDriver("node-001", "unix:///var/run/csi/csi.sock")
	d.mode = DriverModeController
	ids := NewDefaultIdentityServer(d)
	require.Equal(t, d, ids.Driver)

	info, err := ids.GetPluginInfo(context.Background(), &csi.GetPluginInfoRequest{})
	require.NoError(t, err)
	assert.Equal(t, d.name, info.Name)
	assert.Equal(t, d.version, info.VendorVersion)

	probe, err := ids.Probe(context.Background(), &csi.ProbeRequest{})
	require.NoError(t, err)
	assert.NotNil(t, probe)

	caps, err := ids.GetPluginCapabilities(context.Background(), &csi.GetPluginCapabilitiesRequest{})
	require.NoError(t, err)
	require.Len(t, caps.Capabilities, 1)
	assert.Equal(t, csi.PluginCapability_Service_CONTROLLER_SERVICE, caps.Capabilities[0].GetService().GetType())
}

func TestIdentityServer_NodeModeDoesNotAdvertiseControllerService(t *testing.T) {
	d := NewDriver("node-001", "unix:///var/run/csi/csi.sock")
	d.mode = DriverModeNode
	ids := NewDefaultIdentityServer(d)

	caps, err := ids.GetPluginCapabilities(context.Background(), &csi.GetPluginCapabilitiesRequest{})
	require.NoError(t, err)
	assert.Empty(t, caps.Capabilities)
}

func TestIdentityServer_GetPluginInfoErrors(t *testing.T) {
	ids := &IdentityServer{Driver: &driver{version: "0.1.0"}}
	_, err := ids.GetPluginInfo(context.Background(), &csi.GetPluginInfoRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Driver name")

	ids = &IdentityServer{Driver: &driver{name: driverName}}
	_, err = ids.GetPluginInfo(context.Background(), &csi.GetPluginInfoRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "version")
}

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		name      string
		endpoint  string
		wantProto string
		wantAddr  string
		wantErr   bool
	}{
		{name: "unix", endpoint: "unix:///var/run/csi.sock", wantProto: "unix", wantAddr: "/var/run/csi.sock"},
		{name: "tcp", endpoint: "tcp://127.0.0.1:10000", wantProto: "tcp", wantAddr: "127.0.0.1:10000"},
		{name: "upper scheme", endpoint: "TCP://127.0.0.1:10000", wantProto: "TCP", wantAddr: "127.0.0.1:10000"},
		{name: "missing address", endpoint: "unix://", wantErr: true},
		{name: "bad scheme", endpoint: "npipe://pipe/csi", wantErr: true},
		{name: "missing scheme", endpoint: "/var/run/csi.sock", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proto, addr, err := ParseEndpoint(tt.endpoint)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantProto, proto)
			assert.Equal(t, tt.wantAddr, addr)
		})
	}
}

func TestLogGRPC(t *testing.T) {
	info := &grpc.UnaryServerInfo{FullMethod: "/csi.v1.Identity/Probe"}
	resp, err := logGRPC(context.Background(), "request", info, func(ctx context.Context, req interface{}) (interface{}, error) {
		assert.Equal(t, "request", req)
		return "response", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "response", resp)

	expectedErr := errors.New("handler failed")
	resp, err = logGRPC(context.Background(), "request", info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, expectedErr
	})
	assert.ErrorIs(t, err, expectedErr)
	assert.Nil(t, resp)
}

func TestNewNonBlockingGRPCServer(t *testing.T) {
	server := NewNonBlockingGRPCServer()

	assert.NotNil(t, server)
	assert.IsType(t, &nonBlockingGRPCServer{}, server)
}

type fakeNonBlockingGRPCServer struct {
	endpoint string
	ids      csi.IdentityServer
	cs       csi.ControllerServer
	ns       csi.NodeServer
	waited   bool
}

func (f *fakeNonBlockingGRPCServer) Start(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {
	f.endpoint = endpoint
	f.ids = ids
	f.cs = cs
	f.ns = ns
}

func (f *fakeNonBlockingGRPCServer) Wait() {
	f.waited = true
}

func (f *fakeNonBlockingGRPCServer) Stop() {}

func (f *fakeNonBlockingGRPCServer) ForceStop() {}

type validatingMockBackend struct {
	mockBackend
	validateCalls int
	validateErr   error
}

func (m *validatingMockBackend) Validate(ctx context.Context) error {
	m.validateCalls++
	return m.validateErr
}

func TestValidateBackendStartup(t *testing.T) {
	t.Run("skips backend without validator", func(t *testing.T) {
		require.NoError(t, validateBackendStartup(context.Background(), &mockBackend{}))
	})

	t.Run("calls validator", func(t *testing.T) {
		backend := &validatingMockBackend{}

		require.NoError(t, validateBackendStartup(context.Background(), backend))
		assert.Equal(t, 1, backend.validateCalls)
	})

	t.Run("returns validator error", func(t *testing.T) {
		backend := &validatingMockBackend{validateErr: errors.New("probe failed")}

		err := validateBackendStartup(context.Background(), backend)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "probe failed")
		assert.Equal(t, 1, backend.validateCalls)
	})
}

func TestDriverRunWiresBackendAndServers(t *testing.T) {
	backend := &validatingMockBackend{}
	fakeServer := &fakeNonBlockingGRPCServer{}
	t.Setenv(driverRunDirEnv, t.TempDir())

	originalBackendFromEnv := newBackendFromEnvForRun
	originalServerFactory := newNonBlockingGRPCServerForRun
	newBackendFromEnvForRun = func() (Backend, error) {
		return backend, nil
	}
	newNonBlockingGRPCServerForRun = func() NonBlockingGRPCServer {
		return fakeServer
	}
	t.Cleanup(func() {
		newBackendFromEnvForRun = originalBackendFromEnv
		newNonBlockingGRPCServerForRun = originalServerFactory
	})

	d := NewDriver("node-001", "tcp://127.0.0.1:10000")
	d.Run(DriverModeController)

	assert.Same(t, backend, d.backend)
	assert.Equal(t, 1, backend.validateCalls)
	assert.Equal(t, DriverModeController, d.mode)
	assert.Equal(t, "tcp://127.0.0.1:10000", fakeServer.endpoint)
	assert.True(t, fakeServer.waited)
	assert.IsType(t, &IdentityServer{}, fakeServer.ids)
	assert.IsType(t, &ControllerServer{}, fakeServer.cs)
	assert.Nil(t, fakeServer.ns)
}

func TestDriverRunNodeModeDoesNotInitializeBackend(t *testing.T) {
	fakeServer := &fakeNonBlockingGRPCServer{}
	t.Setenv(driverRunDirEnv, t.TempDir())

	originalBackendFromEnv := newBackendFromEnvForRun
	originalServerFactory := newNonBlockingGRPCServerForRun
	newBackendFromEnvForRun = func() (Backend, error) {
		t.Fatal("node mode must not initialize the WinRM backend")
		return nil, nil
	}
	newNonBlockingGRPCServerForRun = func() NonBlockingGRPCServer {
		return fakeServer
	}
	t.Cleanup(func() {
		newBackendFromEnvForRun = originalBackendFromEnv
		newNonBlockingGRPCServerForRun = originalServerFactory
	})

	d := NewDriver("node-001", "tcp://127.0.0.1:10000")
	d.Run(DriverModeNode)

	assert.Nil(t, d.backend)
	assert.Equal(t, DriverModeNode, d.mode)
	assert.Equal(t, "tcp://127.0.0.1:10000", fakeServer.endpoint)
	assert.True(t, fakeServer.waited)
	assert.IsType(t, &IdentityServer{}, fakeServer.ids)
	assert.Nil(t, fakeServer.cs)
	assert.IsType(t, &nodeServer{}, fakeServer.ns)
}

func TestDriverRunControllerModeWithLeaderElection(t *testing.T) {
	backend := &mockBackend{}
	fakeServer := &fakeNonBlockingGRPCServer{}
	t.Setenv(driverRunDirEnv, t.TempDir())

	var captured LeaderElectionConfig
	originalBackendFromEnv := newBackendFromEnvForRun
	originalServerFactory := newNonBlockingGRPCServerForRun
	originalLeaderElection := runLeaderElectionForRun
	newBackendFromEnvForRun = func() (Backend, error) {
		return backend, nil
	}
	newNonBlockingGRPCServerForRun = func() NonBlockingGRPCServer {
		return fakeServer
	}
	runLeaderElectionForRun = func(ctx context.Context, config LeaderElectionConfig, run func(context.Context)) error {
		captured = config
		run(context.Background())
		return nil
	}
	t.Cleanup(func() {
		newBackendFromEnvForRun = originalBackendFromEnv
		newNonBlockingGRPCServerForRun = originalServerFactory
		runLeaderElectionForRun = originalLeaderElection
	})

	d := NewDriver("node-001", "tcp://127.0.0.1:10000")
	d.RunWithOptions(RunOptions{
		Mode: DriverModeController,
		LeaderElection: LeaderElectionConfig{
			Enabled:        true,
			LeaseName:      "csi-controller",
			LeaseNamespace: "kube-system",
			Identity:       "controller-0",
			LeaseDuration:  30 * time.Second,
			RenewDeadline:  20 * time.Second,
			RetryPeriod:    5 * time.Second,
		},
	})

	assert.Equal(t, "csi-controller", captured.LeaseName)
	assert.Equal(t, "kube-system", captured.LeaseNamespace)
	assert.Equal(t, "controller-0", captured.Identity)
	assert.Same(t, backend, d.backend)
	assert.Equal(t, DriverModeController, d.mode)
	assert.IsType(t, &IdentityServer{}, fakeServer.ids)
	assert.IsType(t, &ControllerServer{}, fakeServer.cs)
	assert.Nil(t, fakeServer.ns)
}

func TestDriverRunDirectoryUsesConfiguredBaseDir(t *testing.T) {
	runDir := t.TempDir()
	t.Setenv(driverRunDirEnv, runDir)

	d := NewNamedDriver("nfs.csi.windows.microsoft.com", "node-001", "tcp://127.0.0.1:10000")
	d.ensureRunDirectory()

	info, err := os.Stat(filepath.Join(runDir, "nfs.csi.windows.microsoft.com"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// ---------------------------------------------------------------------------
// volID encode/decode tests
// ---------------------------------------------------------------------------

func TestEncodeDecodeVolID(t *testing.T) {
	original := volID{
		VolumeName:   "k8s-csi-test-volume-001",
		TargetPortal: "10.0.0.1:3260",
		TargetIQN:    "iqn.2024-01.com.example:test-volume",
		LUN:          0,
		VHDXPath:     "D:\\vhdx\\test-volume.vhdx",
		SizeBytes:    1073741824,
	}

	encoded := encodeVolID(original)
	assert.NotEmpty(t, encoded)

	decoded, err := decodeVolID(encoded)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestDecodeVolID_InvalidBase64(t *testing.T) {
	_, err := decodeVolID("not-valid-base64!!!")
	assert.Error(t, err)
}

func TestDecodeVolID_EmptyString(t *testing.T) {
	v, err := decodeVolID("")
	assert.Error(t, err)
	assert.Equal(t, volID{}, v)
}

func TestDecodeVolID_MalformedJSON(t *testing.T) {
	_, err := decodeVolID("eyJrZXkiOiAidmFsdWUifQ==") // valid base64 but not JSON
	assert.Error(t, err)
}

func TestDecodeVolID_InvalidJSON(t *testing.T) {
	_, err := decodeVolID("aW52YWxpZCBqc29u") // "invalid json" in base64
	assert.Error(t, err)
}

func TestEncodeDecode_SpecialChars(t *testing.T) {
	original := volID{
		VolumeName:   "vol-with-special_chars.123",
		TargetPortal: "192.168.1.100:3260",
		TargetIQN:    "iqn.2024-01.com.example:special-vol!@#",
		LUN:          255,
		VHDXPath:     "C:\\Program Files\\k8s\\csi\\vol.vhdx",
		SizeBytes:    999999999999,
	}

	encoded := encodeVolID(original)
	decoded, err := decodeVolID(encoded)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

// ---------------------------------------------------------------------------
// snapID encode/decode tests
// ---------------------------------------------------------------------------

func TestEncodeDecodeSnapID(t *testing.T) {
	original := snapID{
		SnapshotID:   "snap-001",
		OriginalPath: "D:\\vhdx\\test-volume.vhdx",
	}

	encoded := encodeSnapID(original)
	assert.NotEmpty(t, encoded)

	decoded, err := decodeSnapID(encoded)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestDecodeSnapID_InvalidBase64(t *testing.T) {
	_, err := decodeSnapID("not-valid-base64!!!")
	assert.Error(t, err)
}

func TestDecodeSnapID_EmptyString(t *testing.T) {
	s, err := decodeSnapID("")
	assert.Error(t, err)
	assert.Equal(t, snapID{}, s)
}

func TestEncodeDecodeSnapID_SpecialChars(t *testing.T) {
	original := snapID{
		SnapshotID:   "snap-with-special_chars.123",
		OriginalPath: "C:\\Program Files\\k8s\\csi\\snap.vhdx",
	}

	encoded := encodeSnapID(original)
	decoded, err := decodeSnapID(encoded)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

// ---------------------------------------------------------------------------
// getStr helper test
// ---------------------------------------------------------------------------

func TestGetStr(t *testing.T) {
	tests := []struct {
		name    string
		m       map[string]string
		key     string
		wantVal string
		wantOK  bool
	}{
		{
			name:    "key exists",
			m:       map[string]string{"key": "value"},
			key:     "key",
			wantVal: "value",
			wantOK:  true,
		},
		{
			name:    "key does not exist",
			m:       map[string]string{"key": "value"},
			key:     "other",
			wantVal: "",
			wantOK:  false,
		},
		{
			name:    "empty map",
			m:       map[string]string{},
			key:     "key",
			wantVal: "",
			wantOK:  false,
		},
		{
			name:    "nil map",
			m:       nil,
			key:     "key",
			wantVal: "",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := getStr(tt.m, tt.key)
			assert.Equal(t, tt.wantVal, val)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}

// ---------------------------------------------------------------------------
// ensureFile tests
// ---------------------------------------------------------------------------

func TestEnsureFile(t *testing.T) {
	// Test with a temp directory
	tmpDir := t.TempDir()
	targetPath := tmpDir + "/test-file"

	err := ensureFile(targetPath)
	assert.NoError(t, err)

	info, err := os.Stat(targetPath)
	assert.NoError(t, err)
	assert.False(t, info.IsDir())
	assert.Equal(t, int64(0), info.Size())
}

func TestEnsureFile_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := tmpDir + "/test-file"

	// Create the file first
	f, err := os.Create(targetPath)
	assert.NoError(t, err)
	f.Close()

	// Calling again should succeed
	err = ensureFile(targetPath)
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// waitForPath tests
// ---------------------------------------------------------------------------

func TestNodeServer_waitForPath(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)

	// Test with a path that exists (temp dir)
	tmpDir := t.TempDir()

	err := ns.waitForPath(tmpDir, 2*time.Second)
	assert.NoError(t, err)
}

func TestNodeServer_waitForPath_NonExistent(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)

	// Test with a non-existent path and very short timeout
	err := ns.waitForPath("/nonexistent/path/that/does/not/exist", 100*time.Millisecond)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// isBlockDevice tests
// ---------------------------------------------------------------------------

func TestIsBlockDevice(t *testing.T) {
	// Test with a temp file (not a block device)
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/test-file"
	err := os.WriteFile(tmpFile, []byte("test"), 0o644)
	assert.NoError(t, err)

	isBlock, err := isBlockDevice(tmpFile)
	assert.NoError(t, err)
	assert.False(t, isBlock)
}

func TestIsBlockDevice_NonExistent(t *testing.T) {
	isBlock, err := isBlockDevice("/nonexistent/device")
	assert.NoError(t, err)
	assert.False(t, isBlock)
}

package iscsilib

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type iscsiCmdCall struct {
	command string
	args    []string
	timeout time.Duration
}

func captureExecWithTimeout(t *testing.T, outputs []string, errs []error) *[]iscsiCmdCall {
	t.Helper()

	calls := []iscsiCmdCall{}
	originalExecWithTimeout := execWithTimeout
	execWithTimeout = func(command string, args []string, timeout time.Duration) ([]byte, error) {
		callArgs := append([]string(nil), args...)
		calls = append(calls, iscsiCmdCall{command: command, args: callArgs, timeout: timeout})
		idx := len(calls) - 1
		var out string
		if idx < len(outputs) {
			out = outputs[idx]
		}
		var err error
		if idx < len(errs) {
			err = errs[idx]
		}
		return []byte(out), err
	}
	t.Cleanup(func() { execWithTimeout = originalExecWithTimeout })
	return &calls
}

func TestIscsiAdmWrappers(t *testing.T) {
	t.Run("list interfaces", func(t *testing.T) {
		calls := captureExecWithTimeout(t, []string{"default\ntcp-iface\n"}, nil)

		got, err := ListInterfaces()
		require.NoError(t, err)
		assert.Equal(t, []string{"default", "tcp-iface", ""}, got)
		require.Len(t, *calls, 1)
		assert.Equal(t, "iscsiadm", (*calls)[0].command)
		assert.Equal(t, []string{"-m", "iface", "-o", "show"}, (*calls)[0].args)
		assert.Equal(t, 3*time.Second, (*calls)[0].timeout)
	})

	t.Run("show interface", func(t *testing.T) {
		calls := captureExecWithTimeout(t, []string{"iface.transport_name = tcp\n"}, nil)

		got, err := ShowInterface("iface0")
		require.NoError(t, err)
		assert.Contains(t, got, "transport_name")
		require.Len(t, *calls, 1)
		assert.Equal(t, []string{"-m", "iface", "-o", "show", "-I", "iface0"}, (*calls)[0].args)
	})

	t.Run("get sessions", func(t *testing.T) {
		calls := captureExecWithTimeout(t, []string{"tcp: [1] 10.0.0.1:3260,1 iqn.2024-01.com.example:vol\n"}, nil)

		got, err := GetSessions()
		require.NoError(t, err)
		assert.Contains(t, got, "iqn.2024-01")
		require.Len(t, *calls, 1)
		assert.Equal(t, []string{"-m", "session"}, (*calls)[0].args)
	})

	t.Run("logout", func(t *testing.T) {
		calls := captureExecWithTimeout(t, nil, nil)

		require.NoError(t, Logout("iqn.2024-01.com.example:vol", "10.0.0.1:3260"))
		require.Len(t, *calls, 1)
		assert.Equal(t, []string{"-m", "node", "-T", "iqn.2024-01.com.example:vol", "-p", "10.0.0.1:3260", "-u"}, (*calls)[0].args)
	})

	t.Run("logout ignores already absent session", func(t *testing.T) {
		calls := captureExecWithTimeout(t, []string{"iscsiadm: No matching sessions found\n"}, []error{errors.New("exit status 21")})

		require.NoError(t, Logout("iqn.2024-01.com.example:vol", "10.0.0.1:3260"))
		require.Len(t, *calls, 1)
		assert.Equal(t, []string{"-m", "node", "-T", "iqn.2024-01.com.example:vol", "-p", "10.0.0.1:3260", "-u"}, (*calls)[0].args)
	})

	t.Run("logout returns command errors", func(t *testing.T) {
		captureExecWithTimeout(t, nil, []error{errors.New(`exec: "iscsiadm": executable file not found`)})

		err := Logout("iqn.2024-01.com.example:vol", "10.0.0.1:3260")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "executable file not found")
	})

	t.Run("delete db entry", func(t *testing.T) {
		calls := captureExecWithTimeout(t, nil, nil)

		require.NoError(t, DeleteDBEntry("iqn.2024-01.com.example:vol"))
		require.Len(t, *calls, 1)
		assert.Equal(t, []string{"-m", "node", "-T", "iqn.2024-01.com.example:vol", "-o", "delete"}, (*calls)[0].args)
	})

	t.Run("delete db entry ignores already absent record", func(t *testing.T) {
		calls := captureExecWithTimeout(t, []string{"iscsiadm: Could not execute operation on all records: encountered iSCSI database failure\n"}, []error{errors.New("exit status 21")})

		require.NoError(t, DeleteDBEntry("iqn.2024-01.com.example:vol"))
		require.Len(t, *calls, 1)
		assert.Equal(t, []string{"-m", "node", "-T", "iqn.2024-01.com.example:vol", "-o", "delete"}, (*calls)[0].args)
	})

	t.Run("delete iface", func(t *testing.T) {
		calls := captureExecWithTimeout(t, nil, nil)

		require.NoError(t, DeleteIFace("iface0"))
		require.Len(t, *calls, 1)
		assert.Equal(t, []string{"-m", "iface", "-I", "iface0", "-o", "delete"}, (*calls)[0].args)
	})
}

func TestCreateDBEntry(t *testing.T) {
	calls := captureExecWithTimeout(t, nil, nil)
	discoverySecrets := Secrets{
		SecretsType: "chap",
		UserName:    "disc-user",
		Password:    "disc-pass",
		UserNameIn:  "disc-user-in",
		PasswordIn:  "disc-pass-in",
	}
	sessionSecrets := Secrets{
		SecretsType: "chap",
		UserName:    "session-user",
		Password:    "session-pass",
		UserNameIn:  "session-user-in",
		PasswordIn:  "session-pass-in",
	}

	err := CreateDBEntry("iqn.2024-01.com.example:vol", "10.0.0.1:3260", "iface0", discoverySecrets, sessionSecrets)
	require.NoError(t, err)
	require.Len(t, *calls, 3)
	assert.Equal(t, []string{"-m", "node", "-T", "iqn.2024-01.com.example:vol", "-p", "10.0.0.1:3260", "-I", "iface0", "-o", "new"}, (*calls)[0].args)
	assert.Contains(t, (*calls)[1].args, "discovery.sendtargets.auth.username")
	assert.Contains(t, (*calls)[1].args, "disc-user-in")
	assert.Contains(t, (*calls)[2].args, "node.session.auth.username")
	assert.Contains(t, (*calls)[2].args, "session-user-in")
}

func TestCreateDBEntryErrors(t *testing.T) {
	t.Run("new entry failure", func(t *testing.T) {
		calls := captureExecWithTimeout(t, nil, []error{errors.New("new failed")})

		err := CreateDBEntry("iqn.2024-01.com.example:vol", "10.0.0.1:3260", "iface0", Secrets{}, Secrets{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "new failed")
		require.Len(t, *calls, 1)
	})

	t.Run("chap failure", func(t *testing.T) {
		calls := captureExecWithTimeout(t, nil, []error{nil, errors.New("chap failed")})

		err := CreateDBEntry("iqn.2024-01.com.example:vol", "10.0.0.1:3260", "iface0", Secrets{SecretsType: "chap"}, Secrets{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update discoverydb with CHAP")
		require.Len(t, *calls, 2)
	})
}

func TestDiscoverydb(t *testing.T) {
	t.Run("success with chap", func(t *testing.T) {
		calls := captureExecWithTimeout(t, nil, nil)

		err := Discoverydb("10.0.0.1:3260", "iface0", Secrets{SecretsType: "chap", UserName: "user", Password: "pass"}, true)
		require.NoError(t, err)
		require.Len(t, *calls, 3)
		assert.Equal(t, []string{"-m", "discoverydb", "-t", "sendtargets", "-p", "10.0.0.1:3260", "-I", "iface0", "-o", "new"}, (*calls)[0].args)
		assert.Contains(t, (*calls)[1].args, "discovery.sendtargets.auth.authmethod")
		assert.Equal(t, []string{"-m", "discoverydb", "-t", "sendtargets", "-p", "10.0.0.1:3260", "-I", "iface0", "--discover"}, (*calls)[2].args)
	})

	t.Run("new failure", func(t *testing.T) {
		calls := captureExecWithTimeout(t, []string{"bad output"}, []error{errors.New("new failed")})

		err := Discoverydb("10.0.0.1:3260", "iface0", Secrets{}, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create new entry")
		assert.Contains(t, err.Error(), "bad output")
		require.Len(t, *calls, 1)
	})

	t.Run("discover failure deletes entry", func(t *testing.T) {
		calls := captureExecWithTimeout(t, nil, []error{nil, errors.New("discover failed"), nil})

		err := Discoverydb("10.0.0.1:3260", "iface0", Secrets{}, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to sendtargets")
		require.Len(t, *calls, 3)
		assert.Equal(t, []string{"-m", "discoverydb", "-t", "sendtargets", "-p", "10.0.0.1:3260", "-I", "iface0", "-o", "delete"}, (*calls)[2].args)
	})
}

func TestConnectorDiscoverTargetWithSessionChapOnly(t *testing.T) {
	calls := captureExecWithTimeout(t, nil, nil)
	conn := Connector{
		DoDiscovery: true,
		SessionSecrets: Secrets{
			SecretsType: "chap",
			UserName:    "session-user",
			Password:    "session-pass",
		},
	}

	require.NoError(t, conn.discoverTarget("iqn.2024-01.com.example:vol", "iface0", "10.0.0.1:3260"))
	require.Len(t, *calls, 4)
	assert.Equal(t, []string{"-m", "discoverydb", "-t", "sendtargets", "-p", "10.0.0.1:3260", "-I", "iface0", "-o", "new"}, (*calls)[0].args)
	assert.Equal(t, []string{"-m", "discoverydb", "-t", "sendtargets", "-p", "10.0.0.1:3260", "-I", "iface0", "--discover"}, (*calls)[1].args)
	assert.Equal(t, []string{"-m", "node", "-T", "iqn.2024-01.com.example:vol", "-p", "10.0.0.1:3260", "-I", "iface0", "-o", "new"}, (*calls)[2].args)
	assert.Contains(t, (*calls)[3].args, "node.session.auth.username")
	assert.Contains(t, (*calls)[3].args, "session-user")
}

func TestLogin(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		calls := captureExecWithTimeout(t, nil, nil)

		require.NoError(t, Login("iqn.2024-01.com.example:vol", "10.0.0.1:3260"))
		require.Len(t, *calls, 1)
		assert.Equal(t, []string{"-m", "node", "-T", "iqn.2024-01.com.example:vol", "-p", "10.0.0.1:3260", "-l"}, (*calls)[0].args)
	})

	t.Run("failure deletes node record", func(t *testing.T) {
		calls := captureExecWithTimeout(t, nil, []error{errors.New("login failed"), nil})

		err := Login("iqn.2024-01.com.example:vol", "10.0.0.1:3260")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to sendtargets")
		require.Len(t, *calls, 2)
		assert.Equal(t, []string{"-m", "node", "-T", "iqn.2024-01.com.example:vol", "-p", "10.0.0.1:3260", "-o", "delete"}, (*calls)[1].args)
	})
}

func TestSessionExists(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		captureExecWithTimeout(t, []string{"tcp: [1] 10.0.0.1:3260,1 iqn.2024-01.com.example:vol\n"}, nil)

		found, err := sessionExists("10.0.0.1:3260", "iqn.2024-01.com.example:vol")
		require.NoError(t, err)
		assert.True(t, found)
	})

	t.Run("not found", func(t *testing.T) {
		captureExecWithTimeout(t, []string{"tcp: [1] 10.0.0.2:3260,1 iqn.2024-01.com.example:other\n"}, nil)

		found, err := sessionExists("10.0.0.1:3260", "iqn.2024-01.com.example:vol")
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("command error", func(t *testing.T) {
		captureExecWithTimeout(t, nil, []error{errors.New("session failed")})

		found, err := sessionExists("10.0.0.1:3260", "iqn.2024-01.com.example:vol")
		require.Error(t, err)
		assert.False(t, found)
	})
}

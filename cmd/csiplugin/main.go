/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/taliesins/csi-driver-for-windows-storage-server/pkg/iscsi"
	klog "k8s.io/klog/v2"
)

var (
	endpoint   = flag.String("endpoint", "unix:///csi/csi.sock", "CSI endpoint")
	nodeID     = flag.String("nodeid", "", "node id")
	driverName = flag.String("drivername", os.Getenv("CSI_DRIVER_NAME"), "CSI driver name")
	mode       = flag.String("mode", os.Getenv("CSI_DRIVER_MODE"), "driver mode: controller or node")

	leaderElectionEnabled       = flag.Bool("leader-election", envBoolDefault("CSI_LEADER_ELECTION", false), "enable Kubernetes Lease leader election; controller mode only")
	leaderElectionName          = flag.String("leader-election-name", os.Getenv("CSI_LEADER_ELECTION_NAME"), "Kubernetes Lease name")
	leaderElectionNamespace     = flag.String("leader-election-namespace", os.Getenv("CSI_LEADER_ELECTION_NAMESPACE"), "Kubernetes Lease namespace")
	leaderElectionIdentity      = flag.String("leader-election-identity", os.Getenv("CSI_LEADER_ELECTION_IDENTITY"), "Kubernetes Lease holder identity")
	leaderElectionLeaseDuration = flag.Duration("leader-election-lease-duration", envDurationDefault("CSI_LEADER_ELECTION_LEASE_DURATION", 15*time.Second), "leader election lease duration")
	leaderElectionRenewDeadline = flag.Duration("leader-election-renew-deadline", envDurationDefault("CSI_LEADER_ELECTION_RENEW_DEADLINE", 10*time.Second), "leader election renew deadline")
	leaderElectionRetryPeriod   = flag.Duration("leader-election-retry-period", envDurationDefault("CSI_LEADER_ELECTION_RETRY_PERIOD", 2*time.Second), "leader election retry period")
)

func init() {
	klog.InitFlags(nil)
}

func main() {
	flag.Parse()
	handle()
	os.Exit(0)
}

func handle() {
	driverMode, err := iscsi.ParseDriverMode(*mode)
	if err != nil {
		klog.Fatalf("invalid --mode: %v", err)
	}

	d := iscsi.NewDriver(*nodeID, *endpoint)
	if *driverName != "" {
		d = iscsi.NewNamedDriver(*driverName, *nodeID, *endpoint)
	}
	d.RunWithOptions(iscsi.RunOptions{
		Mode: driverMode,
		LeaderElection: iscsi.LeaderElectionConfig{
			Enabled:        *leaderElectionEnabled,
			LeaseName:      *leaderElectionName,
			LeaseNamespace: *leaderElectionNamespace,
			Identity:       *leaderElectionIdentity,
			LeaseDuration:  *leaderElectionLeaseDuration,
			RenewDeadline:  *leaderElectionRenewDeadline,
			RetryPeriod:    *leaderElectionRetryPeriod,
		},
	})
}

func envBoolDefault(name string, fallback bool) bool {
	rawValue := os.Getenv(name)
	value := strings.TrimSpace(rawValue)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		klog.V(4).Infof("invalid boolean environment variable %s=%q: %v; using fallback %t", name, rawValue, err, fallback)
		return fallback
	}
	return parsed
}

func envDurationDefault(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

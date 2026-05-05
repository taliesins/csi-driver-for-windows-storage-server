package iscsi

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"
)

var (
	runLeaderElectionForRun   = runLeaderElection
	hostnameForLeaderElection = os.Hostname
)

const (
	defaultLeaderElectionLeaseDuration = 15 * time.Second
	defaultLeaderElectionRenewDeadline = 10 * time.Second
	defaultLeaderElectionRetryPeriod   = 2 * time.Second
)

func (config LeaderElectionConfig) withDefaults(d *driver) (LeaderElectionConfig, error) {
	config.LeaseName = strings.TrimSpace(config.LeaseName)
	if config.LeaseName == "" {
		config.LeaseName = d.name
	}

	config.LeaseNamespace = strings.TrimSpace(config.LeaseNamespace)
	if config.LeaseNamespace == "" {
		config.LeaseNamespace = strings.TrimSpace(os.Getenv("POD_NAMESPACE"))
	}
	if config.LeaseNamespace == "" {
		return LeaderElectionConfig{}, fmt.Errorf("leader election namespace is required")
	}

	config.Identity = strings.TrimSpace(config.Identity)
	if config.Identity == "" {
		config.Identity = strings.TrimSpace(os.Getenv("POD_NAME"))
	}
	if config.Identity == "" {
		hostname, err := hostnameForLeaderElection()
		if err != nil {
			return LeaderElectionConfig{}, fmt.Errorf("get hostname for leader election identity: %w", err)
		}
		config.Identity = strings.TrimSpace(hostname)
	}
	if config.Identity == "" {
		return LeaderElectionConfig{}, fmt.Errorf("leader election identity is required")
	}

	if config.LeaseDuration == 0 {
		config.LeaseDuration = defaultLeaderElectionLeaseDuration
	}
	if config.RenewDeadline == 0 {
		config.RenewDeadline = defaultLeaderElectionRenewDeadline
	}
	if config.RetryPeriod == 0 {
		config.RetryPeriod = defaultLeaderElectionRetryPeriod
	}
	if config.LeaseDuration <= 0 || config.RenewDeadline <= 0 || config.RetryPeriod <= 0 {
		return LeaderElectionConfig{}, fmt.Errorf("leader election durations must be positive")
	}
	if config.LeaseDuration <= config.RenewDeadline {
		return LeaderElectionConfig{}, fmt.Errorf("leader election lease duration must be greater than renew deadline")
	}
	if config.RenewDeadline <= config.RetryPeriod {
		return LeaderElectionConfig{}, fmt.Errorf("leader election renew deadline must be greater than retry period")
	}

	return config, nil
}

func runLeaderElection(ctx context.Context, config LeaderElectionConfig, run func(context.Context)) error {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("build in-cluster Kubernetes client config: %w", err)
	}
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create Kubernetes client: %w", err)
	}

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      config.LeaseName,
			Namespace: config.LeaseNamespace,
		},
		Client: client.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: config.Identity,
		},
	}

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:            lock,
		LeaseDuration:   config.LeaseDuration,
		RenewDeadline:   config.RenewDeadline,
		RetryPeriod:     config.RetryPeriod,
		ReleaseOnCancel: true,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: run,
			OnStoppedLeading: func() {
				klog.Fatalf("lost leadership for lease %s/%s", config.LeaseNamespace, config.LeaseName)
			},
			OnNewLeader: func(identity string) {
				if identity == config.Identity {
					klog.Infof("acquired leadership for lease %s/%s", config.LeaseNamespace, config.LeaseName)
					return
				}
				klog.Infof("leader elected for lease %s/%s: %s", config.LeaseNamespace, config.LeaseName, identity)
			},
		},
	})
	return nil
}

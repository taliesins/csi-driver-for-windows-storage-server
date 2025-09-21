module github.com/taliesins/csi-driver-iscsi-for-windows

go 1.24.0

require (
	github.com/container-storage-interface/spec v1.11.0
	github.com/kubernetes-csi/csi-lib-utils v0.22.0
	golang.org/x/net v0.44.0
	google.golang.org/grpc v1.75.1
	k8s.io/klog/v2 v2.130.1
	k8s.io/mount-utils v0.34.1
)

require (
	github.com/Azure/go-ntlmssp v0.0.0-20221128193559-754e69321358 // indirect
	github.com/ChrisTrenkamp/goxpath v0.0.0-20210404020558-97928f7e12b6 // indirect
	github.com/bodgit/ntlmssp v0.0.0-20240506230425-31973bb52d9b // indirect
	github.com/bodgit/windows v1.0.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/gofrs/uuid v4.4.0+incompatible // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.7.6 // indirect
	github.com/jcmturner/goidentity/v6 v6.0.1 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.4 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/masterzen/simplexml v0.0.0-20190410153822-31eea3082786 // indirect
	github.com/moby/sys/mountinfo v0.7.2 // indirect
	github.com/tidwall/transform v0.0.0-20201103190739-32f242e2dbde // indirect
	golang.org/x/crypto v0.42.0 // indirect
	golang.org/x/text v0.29.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250908214217-97024824d090 // indirect
)

require (
	github.com/masterzen/winrm v0.0.0-20250819055755-20c0798bc988
	golang.org/x/sys v0.36.0
	google.golang.org/protobuf v1.36.9
	k8s.io/utils v0.0.0-20250820121507-0af2bda4dd1d
)

replace k8s.io/api => k8s.io/api v0.29.14

replace k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.29.14

replace k8s.io/apimachinery => k8s.io/apimachinery v0.29.14

replace k8s.io/apiserver => k8s.io/apiserver v0.29.14

replace k8s.io/cli-runtime => k8s.io/cli-runtime v0.29.14

replace k8s.io/client-go => k8s.io/client-go v0.29.14

replace k8s.io/cloud-provider => k8s.io/cloud-provider v0.29.14

replace k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.29.14

replace k8s.io/code-generator => k8s.io/code-generator v0.29.14

replace k8s.io/component-base => k8s.io/component-base v0.29.14

replace k8s.io/component-helpers => k8s.io/component-helpers v0.29.14

replace k8s.io/controller-manager => k8s.io/controller-manager v0.29.14

replace k8s.io/cri-api => k8s.io/cri-api v0.29.14

replace k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.29.14

replace k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.29.14

replace k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.29.14

replace k8s.io/kube-proxy => k8s.io/kube-proxy v0.29.14

replace k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.29.14

replace k8s.io/kubectl => k8s.io/kubectl v0.29.14

replace k8s.io/kubelet => k8s.io/kubelet v0.29.14

replace k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.29.14

replace k8s.io/metrics => k8s.io/metrics v0.29.14

replace k8s.io/mount-utils => k8s.io/mount-utils v0.29.14

replace k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.29.14

replace k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.29.14

replace k8s.io/sample-controller => k8s.io/sample-controller v0.29.14

replace k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.29.14

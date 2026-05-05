module github.com/taliesins/csi-driver-for-windows-storage-server

go 1.26.2

require (
	github.com/container-storage-interface/spec v1.12.0
	github.com/kubernetes-csi/csi-lib-utils v0.23.2
	golang.org/x/net v0.53.0
	google.golang.org/grpc v1.80.0
	k8s.io/klog/v2 v2.140.0
	k8s.io/mount-utils v0.36.0
)

require (
	github.com/Azure/go-ntlmssp v0.1.1 // indirect
	github.com/ChrisTrenkamp/goxpath v0.0.0-20210404020558-97928f7e12b6 // indirect
	github.com/bodgit/ntlmssp v0.0.0-20240506230425-31973bb52d9b // indirect
	github.com/bodgit/windows v1.0.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/emicklei/go-restful/v3 v3.13.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/gofrs/uuid v4.4.0+incompatible // indirect
	github.com/google/gnostic-models v0.7.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.7.6 // indirect
	github.com/jcmturner/goidentity/v6 v6.0.1 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.4 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/masterzen/simplexml v0.0.0-20190410153822-31eea3082786 // indirect
	github.com/moby/sys/mountinfo v0.7.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/tidwall/transform v0.0.0-20201103190739-32f242e2dbde // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/oauth2 v0.34.0 // indirect
	golang.org/x/term v0.42.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	golang.org/x/time v0.14.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260420184626-e10c466a9529 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.13.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/api v0.36.0 // indirect
	k8s.io/kube-openapi v0.0.0-20260317180543-43fb72c5454a // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.2 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
)

require (
	github.com/masterzen/winrm v0.0.0-20260407182533-5570be7f80cf
	github.com/stretchr/testify v1.11.1
	golang.org/x/sys v0.43.0
	// Required by k8s.io/client-go v0.36.0; forcing v1.36.11 downgrades the Kubernetes modules to v0.36.0-alpha.2.
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af
	k8s.io/apimachinery v0.36.0
	k8s.io/client-go v0.36.0
	k8s.io/utils v0.0.0-20260319190234-28399d86e0b5
)

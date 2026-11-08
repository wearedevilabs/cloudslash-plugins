# CloudSlash Kubernetes Provider Plugin

Official Kubernetes provider plugin for CloudSlash. Collects pod resource requests/limits, persistent volumes (PVs/PVCs), orphan LoadBalancer services, and node capacity to detect over-provisioned or unallocated cluster resources.

## Capabilities
- **Provider**: `k8s`
- **Capabilities**: `scan`, `quarantine`, `remediation`
- **Contract**: CloudSlash Plugin SDK v2

## Authentication
Supports standard Kubernetes client authentication:
- In-cluster ServiceAccount token (`/var/run/secrets/kubernetes.io/serviceaccount`)
- Local kubeconfig (`~/.kube/config` or `KUBECONFIG` env var)

## Standalone Build & Run
```bash
go build -o cloudslash-plugin-k8s .
./cloudslash-plugin-k8s
```

## Detaching / Extraction
This plugin is completely decoupled from CloudSlash core. To extract this plugin into its own standalone repository:
```bash
mkdir -p ../cloudslash-plugin-k8s
cp -r ./* ../cloudslash-plugin-k8s/
cd ../cloudslash-plugin-k8s
go mod init github.com/DrSkyle/cloudslash-plugin-k8s
go mod tidy
```

# CloudSlash AWS Provider Plugin

Official Amazon Web Services (AWS) provider plugin for CloudSlash. Collects EC2, EBS, RDS, S3, EIP, and NAT Gateway infrastructure resources and executes least-privilege remediation.

## Capabilities
- **Provider**: `aws`
- **Capabilities**: `scan`, `quarantine`, `remediation`, `permissions`
- **Contract**: CloudSlash Plugin SDK v2

## Required IAM Permissions
The plugin defines least-privilege IAM requirements in `manifest.yaml`.
Generate a least-privilege IAM policy:
```bash
cloudslash permissions --plugin aws --format json
```

## Standalone Build & Run
```bash
go build -o cloudslash-plugin-aws .
./cloudslash-plugin-aws
```

## Detaching / Extraction
This plugin is completely decoupled from CloudSlash core. To extract this plugin into its own standalone repository:
```bash
mkdir -p ../cloudslash-plugin-aws
cp -r ./* ../cloudslash-plugin-aws/
cd ../cloudslash-plugin-aws
go mod init github.com/DrSkyle/cloudslash-plugin-aws
go mod tidy
```

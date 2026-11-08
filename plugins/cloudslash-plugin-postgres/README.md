# cloudslash-plugin-cloudslash-plugin-postgres

CloudSlash provider plugin for Cloudslash-plugin-postgres.

## Implement Scan

Read provider settings from req.Config and discover resources. Send each batch
through the gRPC stream. Return errors from cloud API calls and stream.Send.
The generated Scan method returns an error until you implement it.

```go
node := &pb.Node{Crn: "crn:...", Cost: 42.0}
return stream.Send(&pb.ScanResponse{Nodes: []*pb.Node{node}})
```

## Build and verify

```bash
make build    # compiles the plugin binary
make install  # copies binary and plugin.json to ~/.cloudslash/plugins/cloudslash-plugin-postgres/
```

Install the binary into ~/.cloudslash/plugins/ (or your project's plugins/
directory). Use the scan flag --allow-unsigned for local development until
your release manifest has a trusted signature.

## Architecture

This plugin uses HashiCorp go-plugin (the same framework Terraform uses) to
communicate with the CloudSlash engine over gRPC.

```
Engine                           Plugin binary
  │  Handshake()  ──────────────►  │
  │  Scan()       ──────────────►  │  provider discovery
  │               ◄─ ScanResponse  │
```

See the [Plugin Development Guide](https://docs.cloudslash.dev/how-to/writing-custom-plugins/)
for the full gRPC protocol specification.

## Registry publishing

Update plugin.json with the real release asset URLs and SHA-256 hashes. Sign
the manifest with the registry key before publishing. Users can install a
community plugin directly from its repository with a github:owner/repo ref.

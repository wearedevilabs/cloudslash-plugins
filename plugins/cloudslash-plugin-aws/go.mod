module github.com/DrSkyle/cloudslash-plugin-aws

go 1.26.3

replace github.com/DrSkyle/cloudslash-plugin-sdk => ../../cloudslash/sdk

require (
	github.com/DrSkyle/cloudslash-plugin-sdk v0.0.0-00010101000000-000000000000
	github.com/aws/aws-sdk-go-v2 v1.46.0
	github.com/aws/aws-sdk-go-v2/config v1.32.25
	github.com/aws/aws-sdk-go-v2/service/applicationautoscaling v1.41.10
	github.com/aws/aws-sdk-go-v2/service/cloudwatch v1.53.1
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.54.0
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.281.0
	github.com/aws/aws-sdk-go-v2/service/ecs v1.71.0
	github.com/aws/aws-sdk-go-v2/service/eks v1.77.0
	github.com/aws/aws-sdk-go-v2/service/elasticache v1.51.9
	github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2 v1.54.6
	github.com/aws/aws-sdk-go-v2/service/lambda v1.87.1
	github.com/aws/aws-sdk-go-v2/service/rds v1.114.0
	github.com/aws/aws-sdk-go-v2/service/redshift v1.62.0
	github.com/aws/aws-sdk-go-v2/service/route53 v1.62.1
	github.com/aws/aws-sdk-go-v2/service/s3 v1.104.0
	github.com/aws/aws-sdk-go-v2/service/sts v1.43.3
	github.com/aws/aws-sdk-go-v2/service/wafv2 v1.82.0
	github.com/aws/smithy-go v1.28.1
	github.com/hashicorp/go-plugin v1.7.0
	gopkg.in/ini.v1 v1.67.2
)

require (
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.13 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.19.24 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.29 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.2 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.2 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.30 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.22 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.11.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.29 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.29 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.2.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.31.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.36.6 // indirect
	github.com/fatih/color v1.19.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/oklog/run v1.1.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.39.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260610212136-7ab31c22f7ad // indirect
	google.golang.org/grpc v1.81.1 // indirect
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af // indirect
)

// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package aws

import "github.com/DrSkyle/cloudslash-plugin-sdk/shared/crn"

// toCRN uses the shared wire contract so ARN IDs containing colons remain
// a single escaped CRN segment.
func toCRN(region, account, service, typ, id string) string {
	return crn.New("aws", region, service, typ, account, id).String()
}

func parseCRN(raw string) (region, service, typ, account, id string) {
	parsed, err := crn.Parse(raw)
	if err != nil || parsed.SourceName() != "aws" {
		return "", "", "", "", ""
	}
	return parsed.Scope(), parsed.Service, parsed.Type, parsed.Account, parsed.ID
}

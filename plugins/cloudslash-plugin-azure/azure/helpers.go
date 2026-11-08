// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package azure

import crn "github.com/DrSkyle/cloudslash-plugin-sdk/shared/crn"

// toCRN constructs a CRN for an Azure resource.
func toCRN(region, service, typ, id string) crn.CRN {
	return crn.New("azure", region, service, typ, "current", id)
}

// strPtr safely dereferences a *string.
func strPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// parseTags converts Azure tags to a map.
func parseTags(tags map[string]*string) map[string]string {
	out := make(map[string]string, len(tags))
	for k, v := range tags {
		out[k] = strPtr(v)
	}
	return out
}

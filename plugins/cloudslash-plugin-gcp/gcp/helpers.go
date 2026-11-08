// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package gcp

import crn "github.com/DrSkyle/cloudslash-plugin-sdk/shared/crn"

// toCRN constructs a CRN for a GCP resource.
func toCRN(region, service, typ, id string) crn.CRN {
	return crn.New("gcp", region, service, typ, "current", id)
}

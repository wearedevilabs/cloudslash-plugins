// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package aws

import (
	"encoding/json"

	pb "github.com/DrSkyle/cloudslash-plugin-sdk/shared/proto"
)

// EdgeType is a normalized relationship between two resources. It mirrors the
// core graph's edge vocabulary so the engine can rebuild edges without the
// plugin importing any core graph package.
type EdgeType string

const (
	EdgeTypeAttachedTo EdgeType = "AttachedTo"
	EdgeTypeSecuredBy  EdgeType = "SecuredBy"
	EdgeTypeContains   EdgeType = "Contains"
	EdgeTypeRuns       EdgeType = "Runs"
	EdgeTypeFlowsTo    EdgeType = "FlowsTo"
	EdgeTypeGrants     EdgeType = "Grants"
	EdgeTypeUses       EdgeType = "Uses"
	EdgeTypeUnknown    EdgeType = "Unknown"
)

// Node is a normalized resource discovered by a scanner. It carries only a
// string CRN, generic properties, and an estimated monthly cost — no AWS types.
type Node struct {
	CRN        string
	Properties map[string]interface{}
	Cost       float64
}

// Edge links two normalized nodes by CRN with a relationship type.
type Edge struct {
	Source string
	Target string
	Type   EdgeType
}

// Collector accumulates normalized nodes and edges during a scan and converts
// them to the plugin SDK's wire types for streaming. It is deliberately small:
// the engine owns the real graph; the plugin only gathers data.
type Collector struct {
	nodes []*Node
	edges []*Edge
	byCRN map[string]*Node
}

// NewCollector returns an empty collector.
func NewCollector() *Collector {
	return &Collector{byCRN: make(map[string]*Node)}
}

// AddNode records a node with generic properties.
func (c *Collector) AddNode(crn string, props map[string]interface{}) {
	c.AddTypedNode(crn, props, nil)
}

// AddTypedNode records a node. typedData is accepted for API compatibility with
// the legacy scanner surface but is ignored: nodes are always normalized.
func (c *Collector) AddTypedNode(crn string, props map[string]interface{}, _ interface{}) {
	if n, ok := c.byCRN[crn]; ok {
		for k, v := range props {
			n.Properties[k] = v
		}
		return
	}
	n := &Node{CRN: crn, Properties: props}
	c.nodes = append(c.nodes, n)
	c.byCRN[crn] = n
}

// GetNode returns the node for a CRN, or nil if it has not been scanned.
func (c *Collector) GetNode(crn string) *Node {
	return c.byCRN[crn]
}

// AddEdge records an edge with an Unknown relationship (dependency).
func (c *Collector) AddEdge(src, dst string) {
	c.AddTypedEdge(src, dst, EdgeTypeUnknown, 1)
}

// AddTypedEdge records an edge with an explicit relationship type. weight is
// accepted for compatibility but is not required by the wire format.
func (c *Collector) AddTypedEdge(src, dst string, t EdgeType, _ int) {
	c.edges = append(c.edges, &Edge{Source: src, Target: dst, Type: t})
}

// Nodes converts the collected nodes to SDK wire nodes.
func (c *Collector) Nodes() []*pb.Node {
	out := make([]*pb.Node, 0, len(c.nodes))
	for _, n := range c.nodes {
		props, _ := json.Marshal(n.Properties)
		out = append(out, &pb.Node{Crn: n.CRN, Properties: props, Cost: n.Cost})
	}
	return out
}

// Edges converts the collected edges to SDK wire edges.
func (c *Collector) Edges() []*pb.Edge {
	out := make([]*pb.Edge, 0, len(c.edges))
	for _, e := range c.edges {
		out = append(out, &pb.Edge{SourceCrn: e.Source, TargetCrn: e.Target, Relation: string(e.Type)})
	}
	return out
}

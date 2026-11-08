package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	pb "github.com/DrSkyle/cloudslash-plugin-sdk/shared/proto"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestScanPaginatesAndKeepsAccountIdentity(t *testing.T) {
	client := newLinodeClient("test-token")
	calls := 0
	client.http.Transport = fixtureTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatal("missing authorization")
		}
		body := `{"page":1,"pages":1,"results":0,"data":[]}`
		if r.URL.Path == "/v4/linode/instances" {
			page := r.URL.Query().Get("page")
			body = fmt.Sprintf(`{"page":%s,"pages":2,"results":2,"data":[{"id":%s,"region":"us-east","label":"instance"}]}`, page, page)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	var nodes []*pb.Node
	err := scanResources(context.Background(), client, "customer-a", func(batch *pb.ScanResponse) error { nodes = append(nodes, batch.Nodes...); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if calls != 4 || len(nodes) != 2 || !strings.Contains(nodes[0].Crn, "customer-a") {
		t.Fatalf("unexpected inventory: %d calls, %v", calls, nodes)
	}
	if !strings.Contains(string(nodes[0].Properties), `"cost_known":false`) {
		t.Fatal("unknown cost presented as known")
	}
}
func TestScanRetainsPartialErrorAndDoesNotLeakResponseBody(t *testing.T) {
	client := newLinodeClient("test-token")
	client.http.Transport = fixtureTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v4/volumes" {
			return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader("private diagnostic"))}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"page":1,"pages":1,"results":0,"data":[]}`))}, nil
	})
	err := scanResources(context.Background(), client, "customer-a", func(*pb.ScanResponse) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "private diagnostic") {
		t.Fatalf("wrong partial error: %v", err)
	}
}
func TestScanRequiresIdentityBeforeNetwork(t *testing.T) {
	client := newLinodeClient("test-token")
	client.http.Transport = fixtureTransport(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected network"); return nil, nil })
	if err := scanResources(context.Background(), client, "", func(*pb.ScanResponse) error { return nil }); err == nil {
		t.Fatal("missing account accepted")
	}
}

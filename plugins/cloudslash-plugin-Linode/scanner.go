package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/DrSkyle/cloudslash-plugin-sdk/shared/crn"
	pb "github.com/DrSkyle/cloudslash-plugin-sdk/shared/proto"
)

var linodeFamilies = []struct{ path, service, kind string }{
	{"linode/instances", "compute", "instance"},
	{"volumes", "storage", "volume"},
	{"nodebalancers", "network", "load-balancer"},
}

type linodeClient struct {
	token string
	http  *http.Client
}
type linodePage struct {
	Data    []json.RawMessage `json:"data"`
	Page    int               `json:"page"`
	Pages   int               `json:"pages"`
	Results int               `json:"results"`
}

func newLinodeClient(token string) linodeClient {
	return linodeClient{token: token, http: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c linodeClient) page(ctx context.Context, path string, page int) (linodePage, error) {
	if c.token == "" {
		return linodePage{}, errors.New("LINODE_TOKEN is required with read-only resource scopes")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.linode.com/v4/"+path+"?page_size=100&page="+strconv.Itoa(page), nil)
	if err != nil {
		return linodePage{}, err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Accept", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return linodePage{}, fmt.Errorf("Linode %s request failed: %w", path, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return linodePage{}, fmt.Errorf("Linode %s returned HTTP %d; verify token scope or retry after rate limiting", path, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return linodePage{}, errors.New("Linode response unreadable or exceeds 4 MiB")
	}
	var result linodePage
	if err := json.Unmarshal(data, &result); err != nil {
		return linodePage{}, fmt.Errorf("invalid Linode page: %w", err)
	}
	if result.Page != page || result.Pages < 0 || result.Pages > 1000 || result.Results < 0 || len(result.Data) > 100 || (result.Pages == 0 && len(result.Data) > 0) || (result.Pages > 0 && result.Pages < page) {
		return linodePage{}, errors.New("Linode pagination metadata is inconsistent or exceeds scan limits")
	}
	return result, nil
}

func scanResources(ctx context.Context, client linodeClient, account string, send func(*pb.ScanResponse) error) error {
	if account == "" {
		return errors.New("account_id scan configuration or LINODE_ACCOUNT_ID is required for stable resource identities")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	seen := make(map[string]bool)
	var failures error
	for _, family := range linodeFamilies {
		for page := 1; page <= 1000; page++ {
			if err := ctx.Err(); err != nil {
				return errors.Join(failures, err)
			}
			result, err := client.page(ctx, family.path, page)
			if err != nil {
				failures = errors.Join(failures, err)
				break
			}
			batch := &pb.ScanResponse{}
			for _, raw := range result.Data {
				var properties map[string]any
				decoder := json.NewDecoder(bytes.NewReader(raw))
				decoder.UseNumber()
				if err := decoder.Decode(&properties); err != nil || properties == nil {
					return errors.New("invalid Linode resource")
				}
				id, ok := properties["id"].(json.Number)
				number, idErr := strconv.ParseInt(string(id), 10, 64)
				region, regionOK := properties["region"].(string)
				if !ok || idErr != nil || number <= 0 || !regionOK || region == "" {
					return errors.New("Linode resource has no valid identity or region")
				}
				resource := crn.New("linode", region, family.service, family.kind, account, string(id))
				if _, err := crn.Parse(resource.String()); err != nil {
					return fmt.Errorf("invalid Linode resource identity: %w", err)
				}
				if seen[resource.String()] {
					return errors.New("Linode resource changed pagination during scan; retry for a complete inventory")
				}
				if len(seen) >= 100000 {
					return errors.New("Linode scan exceeds 100000 resources")
				}
				seen[resource.String()] = true
				properties["cost_known"] = false
				encoded, err := json.Marshal(properties)
				if err != nil {
					return err
				}
				batch.Nodes = append(batch.Nodes, &pb.Node{Crn: resource.String(), Properties: encoded})
			}
			if len(batch.Nodes) > 0 {
				if err := send(batch); err != nil {
					return err
				}
			}
			if result.Pages == 0 || page >= result.Pages {
				break
			}
		}
	}
	return failures
}

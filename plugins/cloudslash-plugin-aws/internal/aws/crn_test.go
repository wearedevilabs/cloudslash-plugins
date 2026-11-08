package aws

import (
	"testing"

	"github.com/DrSkyle/cloudslash-plugin-sdk/shared/crn"
)

func TestCRNWithARNID(t *testing.T) {
	raw := toCRN("us-east-1", "123", "ecs", "container-instance", "arn:aws:ecs:us-east-1:123:container/x")
	got, err := crn.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "arn:aws:ecs:us-east-1:123:container/x" {
		t.Fatalf("ID = %q", got.ID)
	}
	_, _, _, _, id := parseCRN(raw)
	if id != got.ID {
		t.Fatalf("parsed ID = %q", id)
	}
}

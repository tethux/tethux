package libvirt

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tethux/tethux/virt"
)

func TestDriverReferenceLifetime(t *testing.T) {
	provider, connectErr := New("test:///default")
	if connectErr != nil {
		t.Fatal(connectErr)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	id := ""
	deleted := false
	t.Cleanup(func() {
		if id != "" && !deleted {
			if deleteErr := provider.Delete(context.Background(), id); deleteErr != nil {
				t.Error(deleteErr)
			}
		}
		if closeErr := provider.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	xml := fmt.Sprintf(`<domain type="test"><name>repolint-%s</name><memory unit="KiB">65536</memory><vcpu>1</vcpu><os><type>hvm</type></os><metadata>%s</metadata></domain>`, uuid.NewString(), metadataXML)
	reference, defineErr := provider.conn.DomainDefineXML(xml)
	if defineErr != nil {
		t.Fatal(defineErr)
	}
	var idErr error
	id, idErr = reference.GetUUIDString()
	releaseErr := reference.Free()
	if idErr != nil || releaseErr != nil {
		t.Fatalf("native reference: %v, %v", idErr, releaseErr)
	}
	for _, change := range []struct {
		run  func(context.Context, string) error
		want virt.NodeState
	}{
		{run: provider.Start, want: virt.NodeRunning},
		{run: provider.Suspend, want: virt.NodeSuspended},
		{run: provider.Resume, want: virt.NodeRunning},
		{run: provider.PowerOff, want: virt.NodeStopped},
	} {
		if lifecycleErr := change.run(ctx, id); lifecycleErr != nil {
			t.Fatal(lifecycleErr)
		}
		state, stateErr := provider.State(ctx, id)
		if stateErr != nil || state != change.want {
			t.Fatalf("native state: %q, %v", state, stateErr)
		}
	}
	if deleteErr := provider.Delete(ctx, id); deleteErr != nil {
		t.Fatal(deleteErr)
	}
	deleted = true
}

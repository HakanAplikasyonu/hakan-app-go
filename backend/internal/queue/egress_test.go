package queue

import "testing"

func TestPoolQueues(t *testing.T) {
	if q := PoolQueues(PoolFixedEgress); len(q) != 1 || q[QueueFixedEgress] == 0 {
		t.Fatalf("fixed-egress pool = %v", q)
	}
	if q := PoolQueues(PoolGeneral); q[QueueFixedEgress] != 0 || q[QueueImports] == 0 || q[QueueNotifications] == 0 {
		t.Fatalf("general pool = %v", q)
	}
	if q := PoolQueues(PoolAll); q[QueueFixedEgress] == 0 || q[QueueImports] == 0 {
		t.Fatalf("all pool = %v", q)
	}
	opts := EntegratorSyncOptions()
	found := false
	for _, o := range opts {
		if o.Value() == QueueFixedEgress {
			found = true
		}
	}
	if !found {
		t.Fatal("integrator sync is not routed to the fixed-egress queue")
	}
}

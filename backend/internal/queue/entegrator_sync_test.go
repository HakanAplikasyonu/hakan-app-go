package queue

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// The sweep and the nightly scan register (the cron specs parse; no Redis
// call happens before Run).
func TestRegisterEntegratorSchedule(t *testing.T) {
	s := asynq.NewScheduler(asynq.RedisClientOpt{Addr: "127.0.0.1:1"}, nil)
	if err := RegisterEntegratorSchedule(s); err != nil {
		t.Fatal(err)
	}
}

// A scheduled run carries Scheduled, so the worker records integrator
// failures instead of letting the queue retry them.
func TestScheduledEntegratorPayload(t *testing.T) {
	p := EntegratorSyncPayload{TaxpayerID: 7, ConnectionUUID: uuid.New(), Kind: "incremental", Scheduled: true}
	task, err := NewEntegratorSyncTask(p)
	if err != nil {
		t.Fatal(err)
	}
	var got EntegratorSyncPayload
	if err := json.Unmarshal(task.Payload(), &got); err != nil || got != p {
		t.Fatalf("payload %+v %v", got, err)
	}
	if len(ScheduledEntegratorSyncOptions("incremental")) == 0 || len(ScheduledEntegratorSyncOptions("full_scan")) == 0 {
		t.Fatal("no options")
	}
}

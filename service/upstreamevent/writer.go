package upstreamevent

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type queuedEvent struct {
	event    ProviderEvent
	priority int
}

var (
	configValue    atomic.Value
	eventQueue     chan queuedEvent
	startedWriters bool
)

func init() {
	configValue.Store(LoadConfig())
}

func currentConfig() Config {
	if cfg, ok := configValue.Load().(Config); ok {
		return cfg
	}
	return LoadConfig()
}

func Start() {
	cfg := LoadConfig()
	configValue.Store(cfg)
	if !cfg.Enabled {
		return
	}
	if cfg.WriteMode == writeModeAsync || cfg.WriteMode == writeModeHybrid {
		startAsyncWriter(cfg)
	}
	if cfg.WebhookURL != "" {
		if strings.TrimSpace(cfg.GatewayKey) == "" {
			common.SysLog("upstream event dispatcher disabled: UPSTREAM_EVENT_GATEWAY_KEY is required for TokenOperation")
			return
		}
		go runDispatcher(cfg)
	}
}

func Emit(event ProviderEvent, priority int) {
	cfg := currentConfig()
	if !cfg.Enabled {
		return
	}
	ensureEvent(&event, "")
	if event.EventID == "" || event.EventType == "" {
		return
	}

	switch cfg.WriteMode {
	case writeModeSync:
		writeEventWithTimeout(cfg, event, priority)
	case writeModeAsync:
		enqueueEvent(cfg, event, priority)
	default:
		if priority <= PriorityCritical {
			writeEventWithTimeout(cfg, event, priority)
			return
		}
		enqueueEvent(cfg, event, priority)
	}
}

func startAsyncWriter(cfg Config) {
	if startedWriters {
		return
	}
	startedWriters = true
	size := cfg.AsyncQueueSize
	if size <= 0 {
		size = 10000
	}
	eventQueue = make(chan queuedEvent, size)
	go asyncWriteLoop(cfg)
}

func enqueueEvent(cfg Config, event ProviderEvent, priority int) {
	if eventQueue == nil {
		go func() {
			if err := persistEvent(event, priority); err != nil {
				common.SysLog("failed to write upstream event: " + err.Error())
			}
		}()
		return
	}
	qe := queuedEvent{event: event, priority: priority}
	select {
	case eventQueue <- qe:
		return
	default:
		if cfg.DropLowPriorityWhenFull && priority > PriorityHigh {
			common.SysLog("upstream event queue full, dropping low priority event " + event.EventID)
			return
		}
		go func() {
			if err := persistEvent(event, priority); err != nil {
				common.SysLog("failed to write upstream event after queue overflow: " + err.Error())
			}
		}()
	}
}

func asyncWriteLoop(cfg Config) {
	flushInterval := cfg.AsyncFlushInterval
	if flushInterval <= 0 {
		flushInterval = time.Second
	}
	batchSize := cfg.AsyncFlushBatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	batch := make([]queuedEvent, 0, batchSize)
	flush := func() {
		for _, qe := range batch {
			if err := persistEvent(qe.event, qe.priority); err != nil {
				common.SysLog("failed to write upstream event: " + err.Error())
			}
		}
		batch = batch[:0]
	}

	for {
		select {
		case qe := <-eventQueue:
			batch = append(batch, qe)
			if len(batch) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func writeEventWithTimeout(cfg Config, event ProviderEvent, priority int) {
	timeout := cfg.SyncTimeout
	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}
	done := make(chan error, 1)
	go func() {
		done <- persistEvent(event, priority)
	}()
	select {
	case err := <-done:
		if err != nil {
			common.SysLog("failed to write upstream event: " + err.Error())
		}
	case <-time.After(timeout):
		common.SysLog("upstream event sync write timed out: " + event.EventID)
	}
}

func persistEvent(event ProviderEvent, priority int) error {
	payload, err := common.Marshal(event)
	if err != nil {
		return err
	}
	row := &model.UpstreamEventOutbox{
		EventID:           event.EventID,
		EventType:         event.EventType,
		Status:            model.UpstreamEventStatusPending,
		Priority:          priority,
		SourceSystem:      event.SourceSystem,
		RequestID:         event.RequestID,
		UpstreamRequestID: event.UpstreamRequestID,
		TaskID:            event.TaskID,
		UpstreamTaskID:    event.UpstreamTaskID,
		Payload:           string(payload),
	}
	return model.CreateUpstreamEventOutbox(row)
}

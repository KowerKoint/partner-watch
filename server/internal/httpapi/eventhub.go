package httpapi

import "sync"

type deviceEvent struct {
	Type      string `json:"type"`
	RequestID string `json:"requestId,omitempty"`
	Status    string `json:"status,omitempty"`
	ImageID   string `json:"imageId,omitempty"`
	Failure   string `json:"failure,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

type eventHub struct {
	mutex       sync.RWMutex
	connections map[string]map[chan deviceEvent]struct{}
}

func newEventHub() *eventHub {
	return &eventHub{connections: make(map[string]map[chan deviceEvent]struct{})}
}

func (h *eventHub) subscribe(deviceID string) (<-chan deviceEvent, func()) {
	channel := make(chan deviceEvent, 8)
	h.mutex.Lock()
	if h.connections[deviceID] == nil {
		h.connections[deviceID] = make(map[chan deviceEvent]struct{})
	}
	h.connections[deviceID][channel] = struct{}{}
	h.mutex.Unlock()
	return channel, func() {
		h.mutex.Lock()
		delete(h.connections[deviceID], channel)
		if len(h.connections[deviceID]) == 0 {
			delete(h.connections, deviceID)
		}
		h.mutex.Unlock()
	}
}

func (h *eventHub) online(deviceID string) bool {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	return len(h.connections[deviceID]) > 0
}

func (h *eventHub) publish(deviceID string, event deviceEvent) bool {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	delivered := false
	for channel := range h.connections[deviceID] {
		select {
		case channel <- event:
			delivered = true
		default:
		}
	}
	return delivered
}

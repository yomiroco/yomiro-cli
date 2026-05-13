package connectors

import "sync"

// TargetResolver routes (serviceType, host, port) to the right handler.
type TargetResolver struct {
	mu         sync.RWMutex
	byType     map[string]ServiceHandler
	byTypeDisc map[string]DiscoveryHandler
	generic    ServiceHandler
}

func NewResolver() *TargetResolver {
	return &TargetResolver{
		byType:     map[string]ServiceHandler{},
		byTypeDisc: map[string]DiscoveryHandler{},
	}
}

// Register adds a handler under serviceType. discovery may be nil.
func (r *TargetResolver) Register(serviceType string, h ServiceHandler, d DiscoveryHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byType[serviceType] = h
	if d != nil {
		r.byTypeDisc[serviceType] = d
	}
}

// SetGeneric installs the fallback handler used when no other type matches.
func (r *TargetResolver) SetGeneric(h ServiceHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.generic = h
}

// Resolve returns the handler for the given service type, falling back to generic.
func (r *TargetResolver) Resolve(serviceType, host string, port int) ServiceHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if h, ok := r.byType[serviceType]; ok {
		return h
	}
	return r.generic
}

// DiscoveryHandlers returns the registered discovery handlers, optionally
// filtered to a single service type.
func (r *TargetResolver) DiscoveryHandlers(filter string) []DiscoveryHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if filter != "" {
		if d, ok := r.byTypeDisc[filter]; ok {
			return []DiscoveryHandler{d}
		}
		return nil
	}
	out := make([]DiscoveryHandler, 0, len(r.byTypeDisc))
	for _, d := range r.byTypeDisc {
		out = append(out, d)
	}
	return out
}

// EnabledTypes returns the registered service types (used for the manifest).
func (r *TargetResolver) EnabledTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.byType))
	for k := range r.byType {
		out = append(out, k)
	}
	return out
}

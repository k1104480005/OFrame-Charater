package provider

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
)

// Registry holds the registered provider adapters and the currently active
// one (generation spec 4.1: 可注册多适配器且运行时切换生效; switching preserves
// the previous provider's configuration — the configuration lives in the
// settings store, not in the registry).
type Registry struct {
	mu     sync.RWMutex
	byID   map[string]Provider
	active string
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{byID: make(map[string]Provider)}
}

// Register adds an adapter. Duplicate ids are rejected.
func (r *Registry) Register(p Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p == nil || p.ID() == "" {
		return configErrf("cannot register a provider without an id")
	}
	if _, dup := r.byID[p.ID()]; dup {
		return configErrf("provider %q is already registered", p.ID())
	}
	r.byID[p.ID()] = p
	if r.active == "" {
		r.active = p.ID()
	}
	return nil
}

// SetActive switches the active provider at runtime (settings change).
func (r *Registry) SetActive(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[id]; !ok {
		return configErrf("unknown provider %q", id)
	}
	r.active = id
	return nil
}

// Active returns the currently active provider.
func (r *Registry) Active() (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.active == "" {
		return nil, configErrf("no active provider registered")
	}
	p, ok := r.byID[r.active]
	if !ok {
		return nil, configErrf("active provider %q is not registered", r.active)
	}
	return p, nil
}

// Get returns a registered provider by id.
func (r *Registry) Get(id string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.byID[id]
	if !ok {
		return nil, configErrf("unknown provider %q", id)
	}
	return p, nil
}

// List returns the registered providers sorted by id.
func (r *Registry) List() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Provider, 0, len(r.byID))
	for _, p := range r.byID {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// Replace swaps an already-registered adapter (used when the local config of a
// provider changes: the service rebuilds the adapter with the new key/model).
// The active slot is preserved.
func (r *Registry) Replace(p Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[p.ID()]; !ok {
		return configErrf("provider %q is not registered", p.ID())
	}
	r.byID[p.ID()] = p
	return nil
}

// Remove unregisters a provider (used by tests and future dynamic providers).
// Removing the active provider clears the active slot.
func (r *Registry) Remove(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[id]; !ok {
		return configErrf("unknown provider %q", id)
	}
	delete(r.byID, id)
	if r.active == id {
		r.active = ""
	}
	return nil
}

// NewAdapter constructs the concrete adapter for a provider id with the given
// config and HTTP client (nil → http.DefaultClient). The service uses it to
// (re)build adapters from the persisted local configuration. Custom providers
// (type "compatible") share the OpenAI-compatible adapter; the built-ins keep
// their vendor-specific adapters.
func NewAdapter(id string, cfg ProviderConfig, client *http.Client) (Provider, error) {
	switch cfg.EffectiveType() {
	case ProviderDoubao:
		return NewDoubao(cfg, client), nil
	case ProviderOpenAI:
		return NewOpenAI(cfg, client), nil
	case ProviderAgnes:
		return NewAgnes(cfg, client), nil
	case ProviderTypeCompatible:
		return NewCompatible(cfg, client), nil
	case ProviderTypeAPI:
		// task 2.7: the 自定义 API preset IS the OpenAI-compatible protocol
		// (that is its definition), carried under an explicit type so the
		// provider identity stays distinguishable from the generic compatible
		// type in settings and routing tests.
		return NewCompatible(cfg, client), nil
	case ProviderTypeDashscope:
		// align-framebaker-providers task 2.2: the 百炼 preset gets its own
		// protocol adapter (native image-synthesis + compatible-mode routing).
		// Purely additive — every previously routed type keeps its branch.
		return NewDashscope(cfg, client), nil
	case ProviderTypeGemini:
		// align-framebaker-providers task 2.3: the banana/Gemini preset gets
		// its own generateContent adapter (x-goog-api-key auth, inlineData
		// reference images). It must never silently fall back to the
		// OpenAI-compatible surface — unknown types still error out below.
		return NewGemini(cfg, client), nil
	case ProviderTypeMiniMax:
		// align-framebaker-providers task 2.4: the MiniMax preset gets its
		// own image_generation protocol adapter (Bearer auth, single
		// subject_reference rule, base_resp/status_code envelope). Purely
		// additive — every previously routed type keeps its branch, and a
		// MiniMax provider must never silently speak the compatible protocol.
		return NewMiniMax(cfg, client), nil
	case ProviderTypeVolcengine:
		// align-framebaker-providers task 2.5: the 火山方舟/豆包 preset gets
		// its own Ark protocol adapter (Seedream POST /images/generations,
		// Bearer auth, "size":"WxH", "image" reference list). Purely additive
		// — the ProviderDoubao branch above keeps its exact adapter, and a
		// volcengine provider must never silently speak the compatible or
		// DashScope protocol.
		return NewVolcengine(cfg, client), nil
	case ProviderTypeCLI:
		// align-framebaker-providers task 3.2: the 自定义 CLI preset runs a
		// local executable through an argv array (never a shell). It performs
		// no HTTP calls at all, so the http client is ignored.
		return NewCLI(cfg, client), nil
	}
	return nil, configErrf("unknown provider type %q for %q", cfg.EffectiveType(), id)
}

// DefaultRegistry registers the three built-in adapters with their default
// configurations and activates Doubao (首次生成默认路由到 Doubao).
func DefaultRegistry() *Registry {
	r := NewRegistry()
	_ = r.Register(NewDoubao(DefaultConfig(ProviderDoubao), nil))
	_ = r.Register(NewOpenAI(DefaultConfig(ProviderOpenAI), nil))
	_ = r.Register(NewAgnes(DefaultConfig(ProviderAgnes), nil))
	_ = r.SetActive(DefaultProviderID)
	return r
}

// RegisteredIDs returns the ids of all registered providers.
func (r *Registry) RegisteredIDs() []string {
	ps := r.List()
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.ID())
	}
	return out
}

// String is a small helper for log output.
func (r *Registry) String() string {
	return fmt.Sprintf("provider registry(%d adapters)", len(r.byID))
}

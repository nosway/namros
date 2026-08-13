package storageclass

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/nosway/namros/internal/storage"
)

const (
	DefaultID = "STANDARD"

	BackendLocal = "local"

	RedundancyReplicated  = "replicated"
	RedundancyErasureCode = "erasure-code"

	ParamProfileGeneration = "profile_generation"
	ParamRedundancyBackend = "redundancy_backend"
	ParamReplicaCount      = "replica_count"
	ParamReadQuorum        = "read_quorum"
	ParamWriteQuorum       = "write_quorum"
	ParamDataShards        = "data_shards"
	ParamParityShards      = "parity_shards"
	ParamMinObjectSize     = "min_object_size_bytes"
	ParamCompatibility     = "compatibility"
)

var (
	ErrUnsupported = errors.New("unsupported storage class")
	ErrDisabled    = errors.New("storage class disabled")
	ErrTooSmall    = errors.New("object is too small for storage class")

	defaultCatalog = mustDefaultCatalog()
)

type Definition struct {
	ID                 string
	Backend            string
	Generation         uint64
	RedundancyBackend  string
	ReplicaCount       int
	ReadQuorum         int
	WriteQuorum        int
	DataShards         int
	ParityShards       int
	MinObjectSizeBytes uint64
	Disabled           bool
	Parameters         map[string]string
}

type Catalog struct {
	defaultID   string
	definitions map[string]Definition
}

type ResolveRequest struct {
	RequestedID string
	Fallback    storage.StorageClassSnapshot
	SizeBytes   uint64
	HasSize     bool
}

func NewCatalog(defaultID string, definitions ...Definition) (Catalog, error) {
	out := Catalog{
		defaultID:   NormalizeID(defaultID),
		definitions: make(map[string]Definition, len(definitions)),
	}
	if out.defaultID == "" {
		out.defaultID = DefaultID
	}
	for _, def := range definitions {
		normalized, err := normalizeDefinition(def)
		if err != nil {
			return Catalog{}, err
		}
		if _, exists := out.definitions[normalized.ID]; exists {
			return Catalog{}, fmt.Errorf("%w: duplicate storage class %q", ErrUnsupported, normalized.ID)
		}
		out.definitions[normalized.ID] = normalized
	}
	if _, ok := out.definitions[out.defaultID]; !ok {
		return Catalog{}, fmt.Errorf("%w: default storage class %q is not defined", ErrUnsupported, out.defaultID)
	}
	return out, nil
}

func DefaultCatalog() Catalog {
	return defaultCatalog
}

func DefaultResolver() Catalog {
	return defaultCatalog
}

func mustDefaultCatalog() Catalog {
	catalog, err := NewCatalog(DefaultID, defaultDefinitions()...)
	if err != nil {
		panic(err)
	}
	return catalog
}

func (c Catalog) Resolve(req ResolveRequest) (storage.StorageClassSnapshot, error) {
	requestedID := NormalizeID(req.RequestedID)
	if requestedID == "" {
		if req.Fallback.StorageClassID != "" {
			return storage.CloneStorageClassSnapshot(req.Fallback), nil
		}
		requestedID = c.defaultID
	}
	def, ok := c.definitions[requestedID]
	if !ok {
		return storage.StorageClassSnapshot{}, fmt.Errorf("%w: %q", ErrUnsupported, requestedID)
	}
	if def.Disabled {
		return storage.StorageClassSnapshot{}, fmt.Errorf("%w: %q", ErrDisabled, requestedID)
	}
	if req.HasSize && def.MinObjectSizeBytes > 0 && req.SizeBytes < def.MinObjectSizeBytes {
		return storage.StorageClassSnapshot{}, fmt.Errorf("%w: %q requires at least %d bytes", ErrTooSmall, requestedID, def.MinObjectSizeBytes)
	}
	snapshot := def.Snapshot()
	if req.Fallback.Backend != "" {
		snapshot.Backend = req.Fallback.Backend
	}
	return snapshot, nil
}

func (c Catalog) Definition(id string) (Definition, bool) {
	def, ok := c.definitions[NormalizeID(id)]
	if !ok {
		return Definition{}, false
	}
	return cloneDefinition(def), true
}

func (d Definition) Snapshot() storage.StorageClassSnapshot {
	def, err := normalizeDefinition(d)
	if err != nil {
		return storage.StorageClassSnapshot{}
	}
	params := cloneStringMap(def.Parameters)
	if params == nil {
		params = make(map[string]string)
	}
	setUintParam(params, ParamProfileGeneration, def.Generation)
	setStringParam(params, ParamRedundancyBackend, def.RedundancyBackend)
	setIntParam(params, ParamReplicaCount, def.ReplicaCount)
	setIntParam(params, ParamReadQuorum, def.ReadQuorum)
	setIntParam(params, ParamWriteQuorum, def.WriteQuorum)
	setIntParam(params, ParamDataShards, def.DataShards)
	setIntParam(params, ParamParityShards, def.ParityShards)
	setUintParam(params, ParamMinObjectSize, def.MinObjectSizeBytes)
	return storage.StorageClassSnapshot{
		StorageClassID: def.ID,
		Backend:        def.Backend,
		Parameters:     params,
	}
}

func StandardSnapshot() storage.StorageClassSnapshot {
	snapshot, err := DefaultResolver().Resolve(ResolveRequest{})
	if err != nil {
		panic(err)
	}
	return snapshot
}

func ID(snapshot storage.StorageClassSnapshot) string {
	id := NormalizeID(snapshot.StorageClassID)
	if id == "" {
		return DefaultID
	}
	return id
}

func NormalizeID(id string) string {
	return strings.ToUpper(strings.TrimSpace(id))
}

func normalizeDefinition(in Definition) (Definition, error) {
	out := cloneDefinition(in)
	out.ID = NormalizeID(out.ID)
	if out.ID == "" {
		return Definition{}, fmt.Errorf("%w: storage class id is required", ErrUnsupported)
	}
	out.Backend = strings.TrimSpace(out.Backend)
	if out.Backend == "" {
		out.Backend = BackendLocal
	}
	if out.Generation == 0 {
		out.Generation = 1
	}
	out.RedundancyBackend = strings.TrimSpace(out.RedundancyBackend)
	if out.RedundancyBackend == "" {
		out.RedundancyBackend = RedundancyReplicated
	}
	return out, nil
}

func defaultDefinitions() []Definition {
	return []Definition{
		replicated(DefaultID, 3, 2, 2),
		replicated("STANDARD_R2", 2, 2, 2),
		replicated("STANDARD_R3", 3, 2, 2),
		replicated("DURABLE_R4", 4, 3, 3),
		compat("REDUCED_REDUNDANCY"),
		compat("STANDARD_IA"),
		compat("ONEZONE_IA"),
		compat("INTELLIGENT_TIERING"),
		compat("GLACIER"),
		compat("GLACIER_IR"),
		compat("DEEP_ARCHIVE"),
		compat("EXPRESS_ONEZONE"),
		ec("EC_4_2", 4, 2, 16*1024*1024),
		ec("EC_8_3", 8, 3, 32*1024*1024),
		ec("EC_10_4", 10, 4, 64*1024*1024),
	}
}

func replicated(id string, replicas, readQuorum, writeQuorum int) Definition {
	return Definition{
		ID:                id,
		RedundancyBackend: RedundancyReplicated,
		ReplicaCount:      replicas,
		ReadQuorum:        readQuorum,
		WriteQuorum:       writeQuorum,
	}
}

func compat(id string) Definition {
	def := replicated(id, 3, 2, 2)
	def.Parameters = map[string]string{
		ParamCompatibility: "s3",
	}
	return def
}

func ec(id string, dataShards, parityShards int, minObjectSizeBytes uint64) Definition {
	return Definition{
		ID:                 id,
		RedundancyBackend:  RedundancyErasureCode,
		DataShards:         dataShards,
		ParityShards:       parityShards,
		MinObjectSizeBytes: minObjectSizeBytes,
	}
}

func cloneDefinition(in Definition) Definition {
	out := in
	out.Parameters = cloneStringMap(in.Parameters)
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func setStringParam(params map[string]string, key, value string) {
	if value != "" {
		params[key] = value
	}
}

func setIntParam(params map[string]string, key string, value int) {
	if value > 0 {
		params[key] = strconv.Itoa(value)
	}
}

func setUintParam(params map[string]string, key string, value uint64) {
	if value > 0 {
		params[key] = strconv.FormatUint(value, 10)
	}
}

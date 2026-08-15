# Cache Package

High-performance caching for graph nodes, edges, and API surface data with pluggable backends.

## Features

- **In-Memory Cache** (default) - Fast, zero-configuration caching
- **SQLite Cache** (optional) - Persistent caching across sessions
- **Hash-Based Invalidation** - Automatic detection of source code changes
- **TTL Expiration** - Configurable time-to-live policies
- **Efficient Indexing** - Fast lookups by ID, package, or relationship

## Quick Start

### In-Memory Cache (Default)

```go
import "github.com/oisee/vibing-steampunk/pkg/cache"

// Create default in-memory cache
c := cache.NewMemoryCache(cache.DefaultConfig())

// Store a node
node := &cache.Node{
    ID:          "ME.ZCL_MY_CLASS\\ME:METHOD",
    ObjectType:  "CLAS",
    ObjectName:  "ZCL_MY_CLASS",
    Package:     "$ZRAY",
    SourceHash:  "abc123...",
    Valid:       true,
}

err := c.PutNode(ctx, node)

// Retrieve it
retrieved, err := c.GetNode(ctx, node.ID)
```

### SQLite Cache (Persistent)

```go
config := cache.DefaultConfig()
config.Type = "sqlite"
config.Path = ".cache/graph.db"

c, err := cache.NewCache(config)
defer c.Close()

// Use same API as in-memory cache
c.PutNode(ctx, node)
```

## Usage Examples

### Graph Traversal

```go
// Cache nodes
c.PutNode(ctx, &cache.Node{ID: "CLAS.ZCL_A", ...})
c.PutNode(ctx, &cache.Node{ID: "CLAS.ZCL_B", ...})

// Cache relationships
c.PutEdge(ctx, &cache.Edge{
    FromID:   "CLAS.ZCL_A",
    ToID:     "CLAS.ZCL_B",
    EdgeType: "CALLS",
    Source:   "WBCROSSGT",
})

// Query: What does ZCL_A call?
deps, _ := c.GetEdgesFrom(ctx, "CLAS.ZCL_A")

// Query: What calls ZCL_B?
callers, _ := c.GetEdgesTo(ctx, "CLAS.ZCL_B")
```

### API Surface Caching

```go
// Cache discovered APIs
c.PutAPI(ctx, &cache.API{
    Name:        "BAPI_SALESORDER_CREATEFROMDAT2",
    Type:        "F",
    UsageCount:  634,
    Module:      "Sales and Distribution",
})

// Get top APIs
topAPIs, _ := c.GetTopAPIs(ctx, 100)
```

### Change Detection

```go
// Store node with source hash
hash := sha256.Sum256([]byte(sourceCode))
node := &cache.Node{
    ID:         "ME.ZCL_TEST\\ME:METHOD",
    SourceHash: hex.EncodeToString(hash[:]),
    Valid:      true,
}
c.PutNode(ctx, node)

// Later: Check if source changed
newHash := sha256.Sum256([]byte(newSourceCode))
cached, _ := c.GetNode(ctx, node.ID)

if cached.SourceHash != hex.EncodeToString(newHash[:]) {
    // Code changed - invalidate
    c.InvalidateNode(ctx, node.ID, "source code modified")
}
```

### Package-Scoped Queries

```go
// Get all nodes in a package
nodes, _ := c.GetNodesByPackage(ctx, "$ZRAY")

for _, node := range nodes {
    fmt.Printf("%s (%s)\n", node.ObjectName, node.ObjectType)
}
```

## Configuration

### Invalidation Policies

```go
// No expiration (useful for testing)
config.InvalidationPolicy = cache.NoInvalidation

// Lazy: TTL only, no active checks (7 days)
config.InvalidationPolicy = cache.LazyInvalidation

// Balanced: TTL + periodic hash checks (1 day) - RECOMMENDED
config.InvalidationPolicy = cache.BalancedInvalidation

// Aggressive: Always check on read
config.InvalidationPolicy = cache.AggressiveInvalidation

// Custom policy
config.InvalidationPolicy = cache.InvalidationPolicy{
    CheckOnRead:       false,
    CheckPeriod:       1 * time.Hour,
    UseHashCheck:      true,
    UseTTL:            true,
    TTL:               24 * time.Hour,
    InvalidateEdges:   true,
}
```

### Memory Limits

```go
config := cache.DefaultConfig()
config.MaxNodes = 10000  // Limit to 10k nodes
config.MaxEdges = 50000  // Limit to 50k edges
config.MaxAPIs  = 5000   // Limit to 5k APIs
```

## API Reference

### Cache Interface

```go
type Cache interface {
    // Node operations
    PutNode(ctx context.Context, node *Node) error
    GetNode(ctx context.Context, id string) (*Node, error)
    DeleteNode(ctx context.Context, id string) error
    InvalidateNode(ctx context.Context, id string, reason string) error
    GetNodesByPackage(ctx context.Context, pkg string) ([]*Node, error)

    // Edge operations
    PutEdge(ctx context.Context, edge *Edge) error
    GetEdgesFrom(ctx context.Context, fromID string) ([]*Edge, error)
    GetEdgesTo(ctx context.Context, toID string) ([]*Edge, error)
    DeleteEdge(ctx context.Context, fromID, toID, edgeType string) error

    // API operations
    PutAPI(ctx context.Context, api *API) error
    GetAPI(ctx context.Context, name, typ string) (*API, error)
    GetTopAPIs(ctx context.Context, limit int) ([]*API, error)

    // Batch operations
    PutNodes(ctx context.Context, nodes []*Node) error
    PutEdges(ctx context.Context, edges []*Edge) error
    PutAPIs(ctx context.Context, apis []*API) error

    // Management
    Clear(ctx context.Context) error
    Stats(ctx context.Context) (*Stats, error)
    Close() error
}
```

### Data Types

```go
type Node struct {
    ID                 string
    ObjectType         string    // CLAS, PROG, FUNC, etc.
    ObjectName         string
    Package            string
    SourceHash         string    // SHA256 of source
    LastModifiedADT    time.Time
    CachedAt           time.Time
    Valid              bool
    InvalidatedAt      *time.Time
    InvalidationReason string
    Metadata           map[string]interface{}
}

type Edge struct {
    FromID       string
    ToID         string
    EdgeType     string    // CALLS, USES, IMPLEMENTS, etc.
    Source       string    // CROSS, WBCROSSGT
    DiscoveredAt time.Time
    Valid        bool
}

type API struct {
    Name         string
    Type         string    // F, ME, TY, etc.
    Source       string    // CROSS, WBCROSSGT
    UsageCount   int
    UsedByCount  int
    Package      string
    Module       string    // SD, MM, FI, etc.
    Description  string
    IsDeprecated bool
    CachedAt     time.Time
    Valid        bool
}
```

## Performance

### In-Memory Cache

- **PutNode**: ~100ns per operation
- **GetNode**: ~50ns per lookup (O(1) hash map)
- **GetEdgesFrom**: ~100ns + O(n) for n edges
- **Memory**: ~256 bytes per node, ~128 bytes per edge

### SQLite Cache

- **PutNode**: ~10µs per operation (with transaction batching)
- **GetNode**: ~5µs per lookup (with indexes)
- **Storage**: ~500 bytes per node (compressed)

## Testing

```bash
# Run tests
go test ./pkg/cache

# Run tests with coverage
go test -cover ./pkg/cache

# Run benchmarks
go test -bench=. ./pkg/cache

# Run examples
go test -run=Example ./pkg/cache
```

## Dependencies

- **In-Memory**: None (stdlib only)
- **SQLite**: `modernc.org/sqlite` (pure Go; no CGO toolchain required)

## Limitations

### In-Memory Cache
- ❌ Data lost on restart
- ❌ Memory limits apply
- ✅ Fastest performance
- ✅ No dependencies

### SQLite Cache
- ✅ Data persists across restarts
- ✅ Can handle large datasets
- ⚠️ Single process only
- ⚠️ Requires disk space

## Future Enhancements

- **PostgreSQL backend** (multi-process, production)
- **Background invalidation** (periodic hash checking)
- **Compression** (reduce memory usage)
- **TTL cleanup** (automatic expired entry removal)
- **Metrics** (Prometheus integration)

## License

See main project LICENSE

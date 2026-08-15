package cache_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/cache"
)

// Example demonstrates basic in-memory cache usage
func Example_inMemory() {
	ctx := context.Background()

	// Create in-memory cache (default)
	c := cache.NewMemoryCache(cache.DefaultConfig())

	// Store a node
	node := &cache.Node{
		ID:         "ME.ZCL_MY_CLASS\\ME:MY_METHOD",
		ObjectType: "CLAS",
		ObjectName: "ZCL_MY_CLASS",
		Package:    "$ZRAY",
		SourceHash: "abc123",
		Valid:      true,
	}

	err := c.PutNode(ctx, node)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Retrieve it
	retrieved, err := c.GetNode(ctx, node.ID)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Retrieved: %s (%s)\n", retrieved.ObjectName, retrieved.ObjectType)

	// Output:
	// Retrieved: ZCL_MY_CLASS (CLAS)
}

// Example_withSQLite demonstrates SQLite-backed cache
func Example_withSQLite() {
	ctx := context.Background()

	// Create SQLite cache in a portable temporary path.
	tmp, err := os.CreateTemp("", "vsp-cache-*.db")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	path := tmp.Name()
	tmp.Close()
	defer os.Remove(path)

	config := cache.DefaultConfig()
	config.Type = "sqlite"
	config.Path = path

	c, err := cache.NewCache(config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer c.Close()

	// Store a node
	node := &cache.Node{
		ID:         "PROG.ZREPORT",
		ObjectType: "PROG",
		ObjectName: "ZREPORT",
		Package:    "$TMP",
		Valid:      true,
	}

	err = c.PutNode(ctx, node)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Get stats
	stats, _ := c.Stats(ctx)
	fmt.Printf("Nodes: %d, Valid: %d\n", stats.NodeCount, stats.ValidNodeCount)

	// Output:
	// Nodes: 1, Valid: 1
}

// Example_graphTraversal demonstrates caching graph traversal results
func Example_graphTraversal() {
	ctx := context.Background()
	c := cache.NewMemoryCache(cache.DefaultConfig())

	// Cache nodes
	nodes := []*cache.Node{
		{ID: "CLAS.ZCL_A", ObjectType: "CLAS", ObjectName: "ZCL_A", Package: "$ZRAY", Valid: true},
		{ID: "CLAS.ZCL_B", ObjectType: "CLAS", ObjectName: "ZCL_B", Package: "$ZRAY", Valid: true},
		{ID: "FUNC.Z_FUNC", ObjectType: "FUNC", ObjectName: "Z_FUNC", Package: "$ZRAY", Valid: true},
	}

	for _, node := range nodes {
		c.PutNode(ctx, node)
	}

	// Cache edges (relationships)
	edges := []*cache.Edge{
		{FromID: "CLAS.ZCL_A", ToID: "CLAS.ZCL_B", EdgeType: "CALLS", Source: "WBCROSSGT", Valid: true},
		{FromID: "CLAS.ZCL_A", ToID: "FUNC.Z_FUNC", EdgeType: "CALLS", Source: "CROSS", Valid: true},
		{FromID: "CLAS.ZCL_B", ToID: "FUNC.Z_FUNC", EdgeType: "CALLS", Source: "CROSS", Valid: true},
	}

	for _, edge := range edges {
		c.PutEdge(ctx, edge)
	}

	// Query: What does ZCL_A call?
	deps, _ := c.GetEdgesFrom(ctx, "CLAS.ZCL_A")
	fmt.Printf("ZCL_A calls %d objects:\n", len(deps))
	for _, edge := range deps {
		fmt.Printf("  - %s (%s)\n", edge.ToID, edge.EdgeType)
	}

	// Query: What calls Z_FUNC?
	callers, _ := c.GetEdgesTo(ctx, "FUNC.Z_FUNC")
	fmt.Printf("Z_FUNC is called by %d objects:\n", len(callers))
	for _, edge := range callers {
		fmt.Printf("  - %s\n", edge.FromID)
	}

	// Output:
	// ZCL_A calls 2 objects:
	//   - CLAS.ZCL_B (CALLS)
	//   - FUNC.Z_FUNC (CALLS)
	// Z_FUNC is called by 2 objects:
	//   - CLAS.ZCL_A
	//   - CLAS.ZCL_B
}

// Example_hashInvalidation demonstrates hash-based change detection
func Example_hashInvalidation() {
	ctx := context.Background()
	c := cache.NewMemoryCache(cache.DefaultConfig())

	// Original source code
	sourceCode := "METHOD my_method.\n  WRITE: 'Hello'.\nENDMETHOD."
	hash := sha256.Sum256([]byte(sourceCode))
	hashStr := hex.EncodeToString(hash[:])

	// Cache node with hash
	node := &cache.Node{
		ID:         "ME.ZCL_TEST\\ME:MY_METHOD",
		ObjectType: "CLAS",
		ObjectName: "ZCL_TEST",
		SourceHash: hashStr,
		Valid:      true,
	}

	c.PutNode(ctx, node)

	// Simulate code change
	newSourceCode := "METHOD my_method.\n  WRITE: 'Hello World'.\nENDMETHOD."
	newHash := sha256.Sum256([]byte(newSourceCode))
	newHashStr := hex.EncodeToString(newHash[:])

	// Check if changed
	cached, _ := c.GetNode(ctx, node.ID)
	if cached.SourceHash != newHashStr {
		fmt.Println("Source code changed - invalidating cache")
		c.InvalidateNode(ctx, node.ID, "source code modified")
	}

	// Try to get now (should be invalidated)
	_, err := c.GetNode(ctx, node.ID)
	fmt.Printf("Cache entry: %v\n", err)

	// Output:
	// Source code changed - invalidating cache
	// Cache entry: cache: entry invalidated
}

// Example_apiSurface demonstrates caching API surface data
func Example_apiSurface() {
	ctx := context.Background()
	c := cache.NewMemoryCache(cache.DefaultConfig())

	// Cache discovered APIs
	apis := []*cache.API{
		{
			Name:        "BAPI_SALESORDER_CREATEFROMDAT2",
			Type:        "F",
			Source:      "CROSS",
			UsageCount:  634,
			UsedByCount: 156,
			Package:     "SD",
			Module:      "Sales and Distribution",
			Description: "Create sales order",
			Valid:       true,
		},
		{
			Name:        "BAPI_TRANSACTION_COMMIT",
			Type:        "F",
			Source:      "CROSS",
			UsageCount:  1247,
			UsedByCount: 412,
			Package:     "BC",
			Module:      "Basis Components",
			Description: "Commit transaction",
			Valid:       true,
		},
	}

	for _, api := range apis {
		c.PutAPI(ctx, api)
	}

	// Get top APIs
	top, _ := c.GetTopAPIs(ctx, 2)
	fmt.Printf("Top %d APIs:\n", len(top))
	for i, api := range top {
		fmt.Printf("%d. %s (used %d times by %d objects)\n",
			i+1, api.Name, api.UsageCount, api.UsedByCount)
	}

	// Output:
	// Top 2 APIs:
	// 1. BAPI_TRANSACTION_COMMIT (used 1247 times by 412 objects)
	// 2. BAPI_SALESORDER_CREATEFROMDAT2 (used 634 times by 156 objects)
}

// Example_ttlPolicy demonstrates TTL-based expiration
func Example_ttlPolicy() {
	ctx := context.Background()

	// Configure with short TTL
	config := cache.DefaultConfig()
	config.InvalidationPolicy = cache.InvalidationPolicy{
		UseTTL: true,
		TTL:    100 * time.Millisecond,
	}

	c := cache.NewMemoryCache(config)

	// Store node
	node := &cache.Node{
		ID:         "TEST",
		ObjectType: "CLAS",
		ObjectName: "ZCL_TEST",
		Valid:      true,
		CachedAt:   time.Now(),
	}

	c.PutNode(ctx, node)

	// Retrieve immediately (should work)
	_, err := c.GetNode(ctx, "TEST")
	fmt.Printf("Immediate read: %v\n", err)

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Try again (should be expired)
	_, err = c.GetNode(ctx, "TEST")
	fmt.Printf("After TTL: %v\n", err)

	// Output:
	// Immediate read: <nil>
	// After TTL: cache: entry expired
}

// Example_packageScope demonstrates querying by package
func Example_packageScope() {
	ctx := context.Background()
	c := cache.NewMemoryCache(cache.DefaultConfig())

	// Cache nodes from different packages
	nodes := []*cache.Node{
		{ID: "N1", ObjectName: "ZCL_RAY_SPIDER", Package: "$ZRAY", Valid: true},
		{ID: "N2", ObjectName: "ZCL_RAY_GRAPH", Package: "$ZRAY", Valid: true},
		{ID: "N3", ObjectName: "ZCL_LLM_CLIENT", Package: "$ZLLM", Valid: true},
		{ID: "N4", ObjectName: "ZTEST_PROG", Package: "$TMP", Valid: true},
	}

	for _, node := range nodes {
		c.PutNode(ctx, node)
	}

	// Query $ZRAY package only
	zrayNodes, _ := c.GetNodesByPackage(ctx, "$ZRAY")
	fmt.Printf("$ZRAY package contains %d nodes:\n", len(zrayNodes))
	for _, node := range zrayNodes {
		fmt.Printf("  - %s\n", node.ObjectName)
	}

	// Output:
	// $ZRAY package contains 2 nodes:
	//   - ZCL_RAY_SPIDER
	//   - ZCL_RAY_GRAPH
}

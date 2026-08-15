package cache

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteCache is a SQLite-backed implementation of Cache
type SQLiteCache struct {
	db     *sql.DB
	config Config
}

// NewSQLiteCache creates a new SQLite cache
func NewSQLiteCache(config Config) (*SQLiteCache, error) {
	if config.Path == "" {
		config.Path = ".cache/graph.db"
	}
	if err := os.MkdirAll(filepath.Dir(config.Path), 0o750); err != nil {
		return nil, fmt.Errorf("failed to create sqlite directory: %w", err)
	}

	db, err := sql.Open("sqlite", config.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite: %w", err)
	}

	// Initialize schema
	if err := initSQLiteSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to init schema: %w", err)
	}

	return &SQLiteCache{db: db, config: config}, nil
}

func initSQLiteSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS cached_nodes (
		id TEXT PRIMARY KEY,
		object_type TEXT NOT NULL,
		object_name TEXT NOT NULL,
		package TEXT,
		enclosing_type TEXT,
		enclosing_name TEXT,
		source_hash TEXT,
		last_modified_adt INTEGER,
		cached_at INTEGER NOT NULL,
		valid INTEGER DEFAULT 1,
		invalidated_at INTEGER,
		invalidation_reason TEXT,
		metadata TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_object_type_name ON cached_nodes(object_type, object_name);
	CREATE INDEX IF NOT EXISTS idx_package ON cached_nodes(package);
	CREATE INDEX IF NOT EXISTS idx_valid ON cached_nodes(valid);

	CREATE TABLE IF NOT EXISTS cached_edges (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_id TEXT NOT NULL,
		to_id TEXT NOT NULL,
		edge_type TEXT NOT NULL,
		source TEXT NOT NULL,
		discovered_at INTEGER NOT NULL,
		valid INTEGER DEFAULT 1,
		UNIQUE(from_id, to_id, edge_type)
	);

	CREATE INDEX IF NOT EXISTS idx_from_id ON cached_edges(from_id);
	CREATE INDEX IF NOT EXISTS idx_to_id ON cached_edges(to_id);

	CREATE TABLE IF NOT EXISTS cached_apis (
		api_name TEXT NOT NULL,
		api_type TEXT NOT NULL,
		source TEXT NOT NULL,
		usage_count INTEGER DEFAULT 0,
		used_by_count INTEGER DEFAULT 0,
		used_by_list TEXT,
		package TEXT,
		module TEXT,
		component TEXT,
		description TEXT,
		is_deprecated INTEGER DEFAULT 0,
		replacement TEXT,
		cached_at INTEGER NOT NULL,
		valid INTEGER DEFAULT 1,
		PRIMARY KEY (api_name, api_type)
	);

	CREATE INDEX IF NOT EXISTS idx_api_module ON cached_apis(module);
	CREATE INDEX IF NOT EXISTS idx_api_usage ON cached_apis(usage_count DESC);
	`

	_, err := db.Exec(schema)
	return err
}

// PutNode stores a node in SQLite
func (s *SQLiteCache) PutNode(ctx context.Context, node *Node) error {
	if node.CachedAt.IsZero() {
		node.CachedAt = time.Now()
	}

	metadataJSON, _ := json.Marshal(node.Metadata)

	query := `
		INSERT INTO cached_nodes
		(id, object_type, object_name, package, enclosing_type, enclosing_name,
		 source_hash, last_modified_adt, cached_at, valid, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			object_type = excluded.object_type,
			object_name = excluded.object_name,
			package = excluded.package,
			source_hash = excluded.source_hash,
			last_modified_adt = excluded.last_modified_adt,
			cached_at = excluded.cached_at,
			valid = excluded.valid,
			metadata = excluded.metadata
	`

	_, err := s.db.ExecContext(ctx, query,
		node.ID,
		node.ObjectType,
		node.ObjectName,
		node.Package,
		node.EnclosingType,
		node.EnclosingName,
		node.SourceHash,
		node.LastModifiedADT.Unix(),
		node.CachedAt.Unix(),
		boolToInt(node.Valid),
		string(metadataJSON),
	)

	return err
}

// GetNode retrieves a node from SQLite
func (s *SQLiteCache) GetNode(ctx context.Context, id string) (*Node, error) {
	query := `
		SELECT id, object_type, object_name, package, enclosing_type, enclosing_name,
		       source_hash, last_modified_adt, cached_at, valid,
		       invalidated_at, invalidation_reason, metadata
		FROM cached_nodes
		WHERE id = ?
	`

	var node Node
	var metadataJSON string
	var lastModifiedUnix, cachedAtUnix int64
	var invalidatedAtUnix sql.NullInt64
	var validInt int

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&node.ID,
		&node.ObjectType,
		&node.ObjectName,
		&node.Package,
		&node.EnclosingType,
		&node.EnclosingName,
		&node.SourceHash,
		&lastModifiedUnix,
		&cachedAtUnix,
		&validInt,
		&invalidatedAtUnix,
		&node.InvalidationReason,
		&metadataJSON,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	node.Valid = intToBool(validInt)
	node.LastModifiedADT = time.Unix(lastModifiedUnix, 0)
	node.CachedAt = time.Unix(cachedAtUnix, 0)

	if invalidatedAtUnix.Valid {
		t := time.Unix(invalidatedAtUnix.Int64, 0)
		node.InvalidatedAt = &t
	}

	if metadataJSON != "" {
		json.Unmarshal([]byte(metadataJSON), &node.Metadata)
	}

	// Check validity and TTL
	if !node.Valid {
		return nil, ErrInvalidated
	}

	if s.config.InvalidationPolicy.UseTTL {
		if time.Since(node.CachedAt) > s.config.InvalidationPolicy.TTL {
			return nil, ErrExpired
		}
	}

	return &node, nil
}

// DeleteNode removes a node from SQLite
func (s *SQLiteCache) DeleteNode(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM cached_nodes WHERE id = ?", id)
	return err
}

// InvalidateNode marks a node as invalid
func (s *SQLiteCache) InvalidateNode(ctx context.Context, id string, reason string) error {
	query := `
		UPDATE cached_nodes
		SET valid = 0, invalidated_at = ?, invalidation_reason = ?
		WHERE id = ?
	`

	_, err := s.db.ExecContext(ctx, query, time.Now().Unix(), reason, id)
	return err
}

// GetNodesByPackage returns all nodes in a package
func (s *SQLiteCache) GetNodesByPackage(ctx context.Context, pkg string) ([]*Node, error) {
	query := `
		SELECT id, object_type, object_name, package, enclosing_type, enclosing_name,
		       source_hash, last_modified_adt, cached_at, valid, metadata
		FROM cached_nodes
		WHERE package = ? AND valid = 1
	`

	rows, err := s.db.QueryContext(ctx, query, pkg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*Node
	for rows.Next() {
		var node Node
		var metadataJSON string
		var lastModifiedUnix, cachedAtUnix int64
		var validInt int

		err := rows.Scan(
			&node.ID,
			&node.ObjectType,
			&node.ObjectName,
			&node.Package,
			&node.EnclosingType,
			&node.EnclosingName,
			&node.SourceHash,
			&lastModifiedUnix,
			&cachedAtUnix,
			&validInt,
			&metadataJSON,
		)
		if err != nil {
			return nil, err
		}

		node.Valid = intToBool(validInt)
		node.LastModifiedADT = time.Unix(lastModifiedUnix, 0)
		node.CachedAt = time.Unix(cachedAtUnix, 0)

		if metadataJSON != "" {
			json.Unmarshal([]byte(metadataJSON), &node.Metadata)
		}

		nodes = append(nodes, &node)
	}

	return nodes, rows.Err()
}

// PutEdge stores an edge in SQLite
func (s *SQLiteCache) PutEdge(ctx context.Context, edge *Edge) error {
	if edge.DiscoveredAt.IsZero() {
		edge.DiscoveredAt = time.Now()
	}

	query := `
		INSERT INTO cached_edges
		(from_id, to_id, edge_type, source, discovered_at, valid)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(from_id, to_id, edge_type) DO UPDATE SET
			source = excluded.source,
			discovered_at = excluded.discovered_at,
			valid = excluded.valid
	`

	_, err := s.db.ExecContext(ctx, query,
		edge.FromID,
		edge.ToID,
		edge.EdgeType,
		edge.Source,
		edge.DiscoveredAt.Unix(),
		boolToInt(edge.Valid),
	)

	return err
}

// GetEdgesFrom returns edges from a node
func (s *SQLiteCache) GetEdgesFrom(ctx context.Context, fromID string) ([]*Edge, error) {
	query := `
		SELECT from_id, to_id, edge_type, source, discovered_at, valid
		FROM cached_edges
		WHERE from_id = ? AND valid = 1
	`

	return s.queryEdges(ctx, query, fromID)
}

// GetEdgesTo returns edges to a node
func (s *SQLiteCache) GetEdgesTo(ctx context.Context, toID string) ([]*Edge, error) {
	query := `
		SELECT from_id, to_id, edge_type, source, discovered_at, valid
		FROM cached_edges
		WHERE to_id = ? AND valid = 1
	`

	return s.queryEdges(ctx, query, toID)
}

func (s *SQLiteCache) queryEdges(ctx context.Context, query string, id string) ([]*Edge, error) {
	rows, err := s.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []*Edge
	for rows.Next() {
		var edge Edge
		var discoveredAtUnix int64
		var validInt int

		err := rows.Scan(
			&edge.FromID,
			&edge.ToID,
			&edge.EdgeType,
			&edge.Source,
			&discoveredAtUnix,
			&validInt,
		)
		if err != nil {
			return nil, err
		}

		edge.DiscoveredAt = time.Unix(discoveredAtUnix, 0)
		edge.Valid = intToBool(validInt)

		edges = append(edges, &edge)
	}

	return edges, rows.Err()
}

// DeleteEdge removes an edge
func (s *SQLiteCache) DeleteEdge(ctx context.Context, fromID, toID, edgeType string) error {
	query := `DELETE FROM cached_edges WHERE from_id = ? AND to_id = ? AND edge_type = ?`
	_, err := s.db.ExecContext(ctx, query, fromID, toID, edgeType)
	return err
}

// Remaining methods (PutAPI, GetAPI, etc.) follow similar patterns...
// Simplified implementations for brevity

func (s *SQLiteCache) PutAPI(ctx context.Context, api *API) error {
	// TODO: Implement
	return nil
}

func (s *SQLiteCache) GetAPI(ctx context.Context, name, typ string) (*API, error) {
	// TODO: Implement
	return nil, ErrNotFound
}

func (s *SQLiteCache) GetTopAPIs(ctx context.Context, limit int) ([]*API, error) {
	// TODO: Implement
	return nil, nil
}

// Batch operations
func (s *SQLiteCache) PutNodes(ctx context.Context, nodes []*Node) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, node := range nodes {
		if err := s.PutNode(ctx, node); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteCache) PutEdges(ctx context.Context, edges []*Edge) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, edge := range edges {
		if err := s.PutEdge(ctx, edge); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteCache) PutAPIs(ctx context.Context, apis []*API) error {
	// TODO: Implement
	return nil
}

// Clear removes all entries
func (s *SQLiteCache) Clear(ctx context.Context) error {
	queries := []string{
		"DELETE FROM cached_nodes",
		"DELETE FROM cached_edges",
		"DELETE FROM cached_apis",
	}

	for _, query := range queries {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return err
		}
	}

	return nil
}

// Stats returns cache statistics
func (s *SQLiteCache) Stats(ctx context.Context) (*Stats, error) {
	stats := &Stats{}

	// Count nodes
	s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM cached_nodes").Scan(&stats.NodeCount)
	s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM cached_nodes WHERE valid = 1").Scan(&stats.ValidNodeCount)

	// Count edges
	s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM cached_edges").Scan(&stats.EdgeCount)
	s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM cached_edges WHERE valid = 1").Scan(&stats.ValidEdgeCount)

	// Count APIs
	s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM cached_apis").Scan(&stats.APICount)
	s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM cached_apis WHERE valid = 1").Scan(&stats.ValidAPICount)

	return stats, nil
}

// Close closes the database connection
func (s *SQLiteCache) Close() error {
	return s.db.Close()
}

// Helper functions
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func intToBool(i int) bool {
	return i != 0
}

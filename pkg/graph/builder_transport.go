package graph

import "strings"

// TransportHeader represents a row from E070 (transport request headers).
type TransportHeader struct {
	TRKORR     string // Transport number (e.g., A4HK900123)
	STRKORR    string // Parent request (empty = this is a request, non-empty = this is a task)
	TRFUNCTION string // K=Workbench, W=Customizing, T=ToC, ...
	TRSTATUS   string // D=Modifiable, R=Released, N=Released+imported
	AS4USER    string // Owner
	AS4DATE    string // YYYYMMDD
	AS4TEXT    string // Description
}

// TransportObject represents a row from E071 (transport object list).
type TransportObject struct {
	TRKORR  string // Transport number (may be task or request)
	PGMID   string // R3TR=object level, LIMU=sub-object
	Object  string // CLAS, PROG, FUGR, TABL, DDLS, ...
	ObjName string // Object name
}

// IsRequest returns true if this header is a request (not a task).
func (h *TransportHeader) IsRequest() bool {
	return h.STRKORR == ""
}

// BuildTransportGraph builds a graph from transport headers and objects.
// Tasks are collapsed into their parent request: objects from tasks get
// IN_TRANSPORT edges to the parent request node, not the task.
//
// Only R3TR (object-level) entries from E071 are included by default.
// LIMU (sub-object components like method includes) are skipped to avoid noise.
func BuildTransportGraph(headers []TransportHeader, objects []TransportObject) *Graph {
	g := New()

	// Index: task number → parent request number
	taskToRequest := make(map[string]string)
	// Index: request number → header
	requestHeaders := make(map[string]*TransportHeader)

	for i := range headers {
		h := &headers[i]
		if h.IsRequest() {
			requestHeaders[h.TRKORR] = h
		} else {
			taskToRequest[h.TRKORR] = h.STRKORR
		}
	}

	// Create TR nodes for requests only
	for _, h := range requestHeaders {
		n := &Node{
			ID:   NodeID(NodeTR, h.TRKORR),
			Name: h.TRKORR,
			Type: NodeTR,
		}
		n.SetMeta("trfunction", h.TRFUNCTION)
		n.SetMeta("trstatus", h.TRSTATUS)
		n.SetMeta("as4user", h.AS4USER)
		n.SetMeta("as4date", h.AS4DATE)
		if h.AS4TEXT != "" {
			n.SetMeta("as4text", h.AS4TEXT)
		}
		g.AddNode(n)
	}

	// Create object → TR edges
	for _, obj := range objects {
		// Skip sub-object components (LIMU)
		if strings.ToUpper(obj.PGMID) != "R3TR" {
			continue
		}

		objType := strings.ToUpper(strings.TrimSpace(obj.Object))
		objName := strings.ToUpper(strings.TrimSpace(obj.ObjName))
		if objType == "" || objName == "" {
			continue
		}

		// Resolve task → parent request
		requestNr := obj.TRKORR
		if parent, ok := taskToRequest[requestNr]; ok {
			requestNr = parent
		}

		// Skip if we don't have the parent request header
		trNodeID := NodeID(NodeTR, requestNr)
		if g.GetNode(trNodeID) == nil {
			continue
		}

		// Ensure object node exists
		objNodeID := NodeID(objType, objName)
		g.AddNode(&Node{
			ID:   objNodeID,
			Name: objName,
			Type: objType,
		})

		// Object → TR edge
		e := &Edge{
			From:   objNodeID,
			To:     trNodeID,
			Kind:   EdgeInTransport,
			Source: SourceE071,
		}
		// Carry request metadata on edge for convenience
		if rh, ok := requestHeaders[requestNr]; ok {
			e.SetMeta("as4user", rh.AS4USER)
			e.SetMeta("as4date", rh.AS4DATE)
			e.SetMeta("trfunction", rh.TRFUNCTION)
		}
		g.AddEdge(e)
	}

	return g
}

// MaterializeCoTransported creates explicit CO_TRANSPORTED edges between objects
// that share at least `minCount` transport requests. This makes co-change a
// first-class relationship in the graph, traversable by Impact BFS.
//
// The edges are weaker than CALLS/REFERENCES — they represent historical
// co-movement, not structural dependency. Use EdgeKinds filter in impact
// queries to include or exclude them.
//
// Direction: edges are created in both directions (A→B and B→A) so that
// Impact BFS (which follows InEdges) can discover the relationship from
// either side.
//
// Source is SourceE071 for plain TR-level correlation, or SourceE070A if
// the graph was built with CR-level expansion (caller should set source).
func MaterializeCoTransported(g *Graph, minCount int, source EdgeSource) int {
	if minCount < 1 {
		minCount = 1
	}
	if source == "" {
		source = SourceE071
	}

	// Collect: for each TR node, which object nodes are connected?
	trToObjects := make(map[string][]string)
	for _, e := range g.Edges() {
		if e.Kind == EdgeInTransport {
			trToObjects[e.To] = append(trToObjects[e.To], e.From)
		}
	}

	// Count co-occurrences between object pairs
	type pair struct{ a, b string }
	pairCount := make(map[pair]int)
	pairTRs := make(map[pair][]string)

	for trID, objs := range trToObjects {
		for i := 0; i < len(objs); i++ {
			for j := i + 1; j < len(objs); j++ {
				a, b := objs[i], objs[j]
				if a > b {
					a, b = b, a // canonical order
				}
				p := pair{a, b}
				pairCount[p]++
				pairTRs[p] = append(pairTRs[p], trID)
			}
		}
	}

	// Create edges for pairs meeting the threshold
	added := 0
	for p, count := range pairCount {
		if count < minCount {
			continue
		}
		// Both directions for BFS traversal
		for _, dir := range [][2]string{{p.a, p.b}, {p.b, p.a}} {
			e := &Edge{
				From:   dir[0],
				To:     dir[1],
				Kind:   EdgeCoTransported,
				Source: source,
			}
			e.SetMeta("count", count)
			if len(pairTRs[p]) <= 10 {
				e.SetMeta("transports", pairTRs[p])
			}
			g.AddEdge(e)
			added++
		}
	}

	return added
}

package adt

import (
	"context"
	"fmt"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/datacluster"
)

// LayoutSpec is what a caller asks to be laid over a cluster's objects.
//
//	applog                          BAL_S_MSG over BALDAT buckets (the messages)
//	stxl                            TLINE over STXL, rendered as text
//	ZDEMO_S_HEADER                  one DDIC structure over every object
//	HDR=ZDEMO_S_HEADER,ITEMS=ZDEMO_S_ITEM   a structure per object name
type LayoutSpec struct {
	// Default is the structure for every object not named in ByObject.
	Default string
	// ByObject maps an object name (as the EXPORT named it) to a structure.
	ByObject map[string]string
}

// The two layouts that are not DDIC structures.
const (
	LayoutAppLog = "APPLOG"
	LayoutSTXL   = "STXL"
)

// ParseLayoutSpec reads the --layout / layout argument.
func ParseLayoutSpec(s string) (LayoutSpec, error) {
	spec := LayoutSpec{ByObject: map[string]string{}}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		obj, name, hasObj := strings.Cut(part, "=")
		if !hasObj {
			if spec.Default != "" {
				return spec, fmt.Errorf("layout %q names two defaults, %s and %s", s, spec.Default, part)
			}
			spec.Default = strings.ToUpper(part)
			continue
		}
		obj, name = strings.ToUpper(strings.TrimSpace(obj)), strings.ToUpper(strings.TrimSpace(name))
		if obj == "" || name == "" {
			return spec, fmt.Errorf("layout %q: expected OBJECT=STRUCTURE", part)
		}
		spec.ByObject[obj] = name
	}
	return spec, nil
}

// Empty reports whether nothing was asked for.
func (s LayoutSpec) Empty() bool { return s.Default == "" && len(s.ByObject) == 0 }

// Mode is the special layout in force for the whole cluster, or "".
func (s LayoutSpec) Mode() string {
	switch s.Default {
	case LayoutAppLog, LayoutSTXL:
		return s.Default
	}
	return ""
}

// For returns the structure asked for an object, or "".
func (s LayoutSpec) For(object string) string {
	if name, ok := s.ByObject[strings.ToUpper(object)]; ok {
		return name
	}
	if s.Mode() != "" {
		return ""
	}
	return s.Default
}

// LayoutResolver fetches DDIC layouts once each and lays them over objects.
// A nil client can only apply the built-in layouts.
type LayoutResolver struct {
	client  *Client
	layouts map[string]*datacluster.Layout
	errs    map[string]error
}

// NewLayoutResolver makes a resolver; client may be nil when no system is
// at hand.
func NewLayoutResolver(client *Client) *LayoutResolver {
	return &LayoutResolver{client: client, layouts: map[string]*datacluster.Layout{}, errs: map[string]error{}}
}

// Layout returns the DDIC layout for a structure name.
func (r *LayoutResolver) Layout(ctx context.Context, name string) (*datacluster.Layout, error) {
	name = strings.ToUpper(strings.TrimSpace(name))
	if name == "TLINE" {
		return datacluster.TLINELayout, nil
	}
	if l, ok := r.layouts[name]; ok {
		return l, nil
	}
	if err, ok := r.errs[name]; ok {
		return nil, err
	}
	if r.client == nil {
		err := fmt.Errorf("layout %s needs a system to read DD03L from; name one with -s", name)
		r.errs[name] = err
		return nil, err
	}
	l, err := r.client.StructureLayout(ctx, name)
	if err != nil {
		r.errs[name] = err
		return nil, err
	}
	r.layouts[name] = l
	return l, nil
}

// Apply lays the spec's structures over every object of the cluster it
// names. Objects that do not fit keep their numbered fields; each failure
// comes back as one note that says which object and why.
func (r *LayoutResolver) Apply(ctx context.Context, c *datacluster.Cluster, spec LayoutSpec) []string {
	var notes []string
	for i := range c.Objects {
		obj := &c.Objects[i]
		name := spec.For(obj.Name)
		if name == "" {
			continue
		}
		l, err := r.Layout(ctx, name)
		if err != nil {
			notes = append(notes, fmt.Sprintf("%s: %v", obj.Name, err))
			continue
		}
		if err := obj.Apply(l); err != nil {
			notes = append(notes, err.Error())
		}
	}
	return notes
}
